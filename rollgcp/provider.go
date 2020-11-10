package rollgcp

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	glBeta "github.com/hashicorp/terraform-provider-google-beta/google-beta"
	"github.com/hashicorp/terraform-provider-google-beta/version"

	googleoauth "golang.org/x/oauth2/google"
)

const TestEnvVar = "TF_ACC"

// Global MutexKV
var mutexKV = glBeta.NewMutexKV()

// Provider returns a *schema.Provider.
func Provider() *schema.Provider {
	provider := &schema.Provider{
		Schema: map[string]*schema.Schema{
			"credentials": {
				Type:     schema.TypeString,
				Optional: true,
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					"GOOGLE_CREDENTIALS",
					"GOOGLE_CLOUD_KEYFILE_JSON",
					"GCLOUD_KEYFILE_JSON",
				}, nil),
				ValidateFunc: validateCredentials,
			},

			"access_token": {
				Type:     schema.TypeString,
				Optional: true,
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					"GOOGLE_OAUTH_ACCESS_TOKEN",
				}, nil),
				ConflictsWith: []string{"credentials"},
			},
			"impersonate_service_account": {
				Type:     schema.TypeString,
				Optional: true,
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					"GOOGLE_IMPERSONATE_SERVICE_ACCOUNT",
				}, nil),
			},

			"impersonate_service_account_delegates": {
				Type:     schema.TypeList,
				Optional: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},

			"project": {
				Type:     schema.TypeString,
				Optional: true,
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					"GOOGLE_PROJECT",
					"GOOGLE_CLOUD_PROJECT",
					"GCLOUD_PROJECT",
					"CLOUDSDK_CORE_PROJECT",
				}, nil),
			},

			"billing_project": {
				Type:     schema.TypeString,
				Optional: true,
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					"GOOGLE_BILLING_PROJECT",
				}, nil),
			},

			"region": {
				Type:     schema.TypeString,
				Optional: true,
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					"GOOGLE_REGION",
					"GCLOUD_REGION",
					"CLOUDSDK_COMPUTE_REGION",
				}, nil),
			},

			"zone": {
				Type:     schema.TypeString,
				Optional: true,
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					"GOOGLE_ZONE",
					"GCLOUD_ZONE",
					"CLOUDSDK_COMPUTE_ZONE",
				}, nil),
			},

			"scopes": {
				Type:     schema.TypeList,
				Optional: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},

			"batching": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"send_after": {
							Type:         schema.TypeString,
							Optional:     true,
							Default:      "10s",
							ValidateFunc: validateNonNegativeDuration(),
						},
						"enable_batching": {
							Type:     schema.TypeBool,
							Optional: true,
							Default:  true,
						},
					},
				},
			},

			"user_project_override": {
				Type:     schema.TypeBool,
				Optional: true,
				DefaultFunc: schema.MultiEnvDefaultFunc([]string{
					"USER_PROJECT_OVERRIDE",
				}, nil),
			},

			"request_timeout": {
				Type:     schema.TypeString,
				Optional: true,
			},

			ComputeBetaCustomEndpointEntryKey:   ComputeBetaCustomEndpointEntry,
			ContainerCustomEndpointEntryKey:     ContainerCustomEndpointEntry,
			ContainerBetaCustomEndpointEntryKey: ContainerBetaCustomEndpointEntry,
		},

		ProviderMetaSchema: map[string]*schema.Schema{
			"module_name": {
				Type:     schema.TypeString,
				Optional: true,
			},
		},

		DataSourcesMap: map[string]*schema.Resource{},

		ResourcesMap: map[string]*schema.Resource{
			"rollgcp_container_node_pool": resourceContainerNodePool(),
		},
	}

	provider.ConfigureContextFunc = func(ctx context.Context, d *schema.ResourceData) (interface{}, diag.Diagnostics) {
		return providerConfigure(ctx, d, provider)
	}

	return provider
}

func providerConfigure(ctx context.Context, d *schema.ResourceData, p *schema.Provider) (interface{}, diag.Diagnostics) {
	config := Config{
		Project:             d.Get("project").(string),
		Region:              d.Get("region").(string),
		Zone:                d.Get("zone").(string),
		UserProjectOverride: d.Get("user_project_override").(bool),
		BillingProject:      d.Get("billing_project").(string),
		userAgent:           p.UserAgent("terraform-provider-google-beta", version.ProviderVersion),
	}

	if v, ok := d.GetOk("request_timeout"); ok {
		var err error
		config.RequestTimeout, err = time.ParseDuration(v.(string))
		if err != nil {
			return nil, diag.FromErr(err)
		}
	}
	// Add credential source
	if v, ok := d.GetOk("access_token"); ok {
		config.AccessToken = v.(string)
	} else if v, ok := d.GetOk("credentials"); ok {
		config.Credentials = v.(string)
	}

	scopes := d.Get("scopes").([]interface{})
	if len(scopes) > 0 {
		config.Scopes = make([]string, len(scopes))
	}
	for i, scope := range scopes {
		config.Scopes[i] = scope.(string)
	}

	batchCfg, err := expandProviderBatchingConfig(d.Get("batching"))
	if err != nil {
		return nil, diag.FromErr(err)
	}
	config.BatchingConfig = batchCfg

	config.ComputeBasePath = ComputeDefaultBasePath
	if value, ok := d.Get("compute_custom_endpoint").(string); ok {
		config.ComputeBasePath = value
	}

	// Handwritten Products / Versioned / Atypical Entries
	config.ComputeBetaBasePath = ComputeBetaDefaultBasePath
	if value, ok := d.Get(ComputeBetaCustomEndpointEntryKey).(string); ok {
		config.ComputeBetaBasePath = value
	}

	config.ContainerBasePath = ContainerDefaultBasePath
	if value, ok := d.Get(ContainerCustomEndpointEntryKey).(string); ok {
		config.ContainerBasePath = value
	}

	config.ContainerBetaBasePath = ContainerBetaDefaultBasePath
	if value, ok := d.Get(ContainerBetaCustomEndpointEntryKey).(string); ok {
		config.ContainerBetaBasePath = value
	}

	stopCtx, ok := schema.StopContext(ctx)
	if !ok {
		stopCtx = ctx
	}
	if err := config.LoadAndValidate(stopCtx); err != nil {
		return nil, diag.FromErr(err)
	}

	return &config, nil
}

func validateCredentials(v interface{}, k string) (warnings []string, errors []error) {
	if v == nil || v.(string) == "" {
		return
	}
	creds := v.(string)
	// if this is a path and we can stat it, assume it's ok
	if _, err := os.Stat(creds); err == nil {
		return
	}
	if _, err := googleoauth.CredentialsFromJSON(context.Background(), []byte(creds)); err != nil {
		errors = append(errors,
			fmt.Errorf("JSON credentials in %q are not valid: %s", creds, err))
	}

	return
}
