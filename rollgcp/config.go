package rollgcp

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"time"

	"github.com/hashicorp/go-cleanhttp"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/logging"
	"google.golang.org/api/option"

	glBeta "github.com/hashicorp/terraform-provider-google-beta/google-beta"
	"golang.org/x/oauth2"
	googleoauth "golang.org/x/oauth2/google"
	computeBeta "google.golang.org/api/compute/v0.beta"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/container/v1"
	containerBeta "google.golang.org/api/container/v1beta1"
)

type providerMeta struct {
	ModuleName string `cty:"module_name"`
}

// Config is the configuration structure used to instantiate the Google
// provider.
type Config struct {
	Credentials         string
	AccessToken         string
	Project             string
	BillingProject      string
	Region              string
	Zone                string
	Scopes              []string
	BatchingConfig      *batchingConfig
	UserProjectOverride bool
	RequestTimeout      time.Duration
	// PollInterval is passed to resource.StateChangeConf in common_operation.go
	// It controls the interval at which we poll for successful operations
	PollInterval time.Duration

	client    *http.Client
	context   context.Context
	userAgent string

	tokenSource oauth2.TokenSource

	ComputeBasePath       string
	ComputeBetaBasePath   string
	ContainerBasePath     string
	ContainerBetaBasePath string

	requestBatcherServiceUsage *RequestBatcher
	requestBatcherIam          *RequestBatcher
}

// Generated product base paths
var ComputeDefaultBasePath = "https://compute.googleapis.com/compute/beta/"

var DefaultClientScopes = []string{
	"https://www.googleapis.com/auth/compute",
	"https://www.googleapis.com/auth/cloud-platform",
	"https://www.googleapis.com/auth/cloud-identity",
	"https://www.googleapis.com/auth/ndev.clouddns.readwrite",
	"https://www.googleapis.com/auth/devstorage.full_control",
	"https://www.googleapis.com/auth/userinfo.email",
}

func (c *Config) LoadAndValidate(ctx context.Context) error {
	if len(c.Scopes) == 0 {
		c.Scopes = DefaultClientScopes
	}

	c.context = ctx

	tokenSource, err := c.getTokenSource(c.Scopes)
	if err != nil {
		return err
	}
	c.tokenSource = tokenSource

	cleanCtx := context.WithValue(ctx, oauth2.HTTPClient, cleanhttp.DefaultClient())

	// 1. OAUTH2 TRANSPORT/CLIENT - sets up proper auth headers
	client := oauth2.NewClient(cleanCtx, tokenSource)

	// 2. Logging Transport - ensure we log HTTP requests to GCP APIs.
	loggingTransport := logging.NewTransport("Google", client.Transport)

	// 3. Retry Transport - retries common temporary errors
	// Keep order for wrapping logging so we log each retried request as well.
	// This value should be used if needed to create shallow copies with additional retry predicates.
	// See ClientWithAdditionalRetries
	retryTransport := NewTransportWithDefaultRetries(loggingTransport)

	// Set final transport value.
	client.Transport = retryTransport

	// This timeout is a timeout per HTTP request, not per logical operation.
	client.Timeout = c.synchronousTimeout()

	c.client = client
	c.context = ctx

	c.Region = glBeta.GetRegionFromRegionSelfLink(c.Region)

	c.requestBatcherServiceUsage = NewRequestBatcher("Service Usage", ctx, c.BatchingConfig)
	c.requestBatcherIam = NewRequestBatcher("IAM", ctx, c.BatchingConfig)

	c.PollInterval = 10 * time.Second

	return nil
}

func expandProviderBatchingConfig(v interface{}) (*batchingConfig, error) {
	config := &batchingConfig{
		sendAfter:      time.Second * defaultBatchSendIntervalSec,
		enableBatching: true,
	}

	if v == nil {
		return config, nil
	}
	ls := v.([]interface{})
	if len(ls) == 0 || ls[0] == nil {
		return config, nil
	}

	cfgV := ls[0].(map[string]interface{})
	if sendAfterV, ok := cfgV["send_after"]; ok {
		sendAfter, err := time.ParseDuration(sendAfterV.(string))
		if err != nil {
			return nil, fmt.Errorf("unable to parse duration from 'send_after' value %q", sendAfterV)
		}
		config.sendAfter = sendAfter
	}

	if enable, ok := cfgV["enable_batching"]; ok {
		config.enableBatching = enable.(bool)
	}

	return config, nil
}

func (c *Config) synchronousTimeout() time.Duration {
	if c.RequestTimeout == 0 {
		return 30 * time.Second
	}
	return c.RequestTimeout
}

func (c *Config) getTokenSource(clientScopes []string) (oauth2.TokenSource, error) {
	creds, err := c.GetCredentials(clientScopes)
	if err != nil {
		return nil, fmt.Errorf("%s", err)
	}
	return creds.TokenSource, nil
}

// Methods to create new services from config
// Some base paths below need the version and possibly more of the path
// set on them. The client libraries are inconsistent about which values they need;
// while most only want the host URL, some older ones also want the version and some
// of those "projects" as well. You can find out if this is required by looking at
// the basePath value in the client library file.
func (c *Config) NewComputeClient(userAgent string) *compute.Service {
	computeClientBasePath := c.ComputeBasePath + "projects/"
	log.Printf("[INFO] Instantiating GCE client for path %s", computeClientBasePath)
	clientCompute, err := compute.NewService(c.context, option.WithHTTPClient(c.client))
	if err != nil {
		log.Printf("[WARN] Error creating client compute: %s", err)
		return nil
	}
	clientCompute.UserAgent = userAgent
	clientCompute.BasePath = computeClientBasePath

	return clientCompute
}

func (c *Config) NewComputeBetaClient(userAgent string) *computeBeta.Service {
	computeBetaClientBasePath := c.ComputeBetaBasePath + "projects/"
	log.Printf("[INFO] Instantiating GCE Beta client for path %s", computeBetaClientBasePath)
	clientComputeBeta, err := computeBeta.NewService(c.context, option.WithHTTPClient(c.client))
	if err != nil {
		log.Printf("[WARN] Error creating client compute beta: %s", err)
		return nil
	}
	clientComputeBeta.UserAgent = userAgent
	clientComputeBeta.BasePath = computeBetaClientBasePath

	return clientComputeBeta
}

func (c *Config) NewContainerClient(userAgent string) *container.Service {
	containerClientBasePath := removeBasePathVersion(c.ContainerBasePath)
	log.Printf("[INFO] Instantiating GKE client for path %s", containerClientBasePath)
	clientContainer, err := container.NewService(c.context, option.WithHTTPClient(c.client))
	if err != nil {
		log.Printf("[WARN] Error creating client container: %s", err)
		return nil
	}
	clientContainer.UserAgent = userAgent
	clientContainer.BasePath = containerClientBasePath

	return clientContainer
}

func (c *Config) NewContainerBetaClient(userAgent string) *containerBeta.Service {
	containerBetaClientBasePath := removeBasePathVersion(c.ContainerBetaBasePath)
	log.Printf("[INFO] Instantiating GKE Beta client for path %s", containerBetaClientBasePath)
	clientContainerBeta, err := containerBeta.NewService(c.context, option.WithHTTPClient(c.client))
	if err != nil {
		log.Printf("[WARN] Error creating client container beta: %s", err)
		return nil
	}
	clientContainerBeta.UserAgent = userAgent
	clientContainerBeta.BasePath = containerBetaClientBasePath

	return clientContainerBeta
}

// staticTokenSource is used to be able to identify static token sources without reflection.
type staticTokenSource struct {
	oauth2.TokenSource
}

func (c *Config) GetCredentials(clientScopes []string) (googleoauth.Credentials, error) {
	if c.AccessToken != "" {
		contents, _, err := pathOrContents(c.AccessToken)
		if err != nil {
			return googleoauth.Credentials{}, fmt.Errorf("Error loading access token: %s", err)
		}

		log.Printf("[INFO] Authenticating using configured Google JSON 'access_token'...")
		log.Printf("[INFO]   -- Scopes: %s", clientScopes)
		token := &oauth2.Token{AccessToken: contents}

		return googleoauth.Credentials{
			TokenSource: staticTokenSource{oauth2.StaticTokenSource(token)},
		}, nil
	}

	if c.Credentials != "" {
		contents, _, err := pathOrContents(c.Credentials)
		if err != nil {
			return googleoauth.Credentials{}, fmt.Errorf("error loading credentials: %s", err)
		}

		creds, err := googleoauth.CredentialsFromJSON(c.context, []byte(contents), clientScopes...)
		if err != nil {
			return googleoauth.Credentials{}, fmt.Errorf("unable to parse credentials from '%s': %s", contents, err)
		}

		log.Printf("[INFO] Authenticating using configured Google JSON 'credentials'...")
		log.Printf("[INFO]   -- Scopes: %s", clientScopes)
		return *creds, nil
	}

	log.Printf("[INFO] Authenticating using DefaultClient...")
	log.Printf("[INFO]   -- Scopes: %s", clientScopes)

	defaultTS, err := googleoauth.DefaultTokenSource(context.Background(), clientScopes...)
	if err != nil {
		return googleoauth.Credentials{}, fmt.Errorf("Attempted to load application default credentials since neither `credentials` nor `access_token` was set in the provider block.  No credentials loaded. To use your gcloud credentials, run 'gcloud auth application-default login'.  Original error: %w", err)
	}
	return googleoauth.Credentials{
		TokenSource: defaultTS,
	}, err
}

// Remove the `/{{version}}/` from a base path if present.
func removeBasePathVersion(url string) string {
	re := regexp.MustCompile(`(?P<base>http[s]://.*)(?P<version>/[^/]+?/$)`)
	return re.ReplaceAllString(url, "$1/")
}

// For a consumer of config.go that isn't a full fledged provider and doesn't
// have its own endpoint mechanism such as sweepers, init {{service}}BasePath
// values to a default. After using this, you should call config.LoadAndValidate.
func ConfigureBasePaths(c *Config) {
	c.ComputeBasePath = ComputeDefaultBasePath
	c.ComputeBetaBasePath = ComputeBetaDefaultBasePath

	// Handwritten Products / Versioned / Atypical Entries
	c.ContainerBasePath = ContainerDefaultBasePath
	c.ContainerBetaBasePath = ContainerBetaDefaultBasePath
}
