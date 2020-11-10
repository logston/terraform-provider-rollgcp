package rollgcp

import (
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

var ComputeBetaDefaultBasePath = "https://www.googleapis.com/compute/beta/"
var ComputeBetaCustomEndpointEntryKey = "compute_beta_custom_endpoint"
var ComputeBetaCustomEndpointEntry = &schema.Schema{
	Type:         schema.TypeString,
	Optional:     true,
	ValidateFunc: validateCustomEndpoint,
	DefaultFunc: schema.MultiEnvDefaultFunc([]string{
		"GOOGLE_COMPUTE_BETA_CUSTOM_ENDPOINT",
	}, ComputeBetaDefaultBasePath),
}

var ContainerDefaultBasePath = "https://container.googleapis.com/v1/"
var ContainerCustomEndpointEntryKey = "container_custom_endpoint"
var ContainerCustomEndpointEntry = &schema.Schema{
	Type:         schema.TypeString,
	Optional:     true,
	ValidateFunc: validateCustomEndpoint,
	DefaultFunc: schema.MultiEnvDefaultFunc([]string{
		"GOOGLE_CONTAINER_CUSTOM_ENDPOINT",
	}, ContainerDefaultBasePath),
}

var ContainerBetaDefaultBasePath = "https://container.googleapis.com/v1beta1/"
var ContainerBetaCustomEndpointEntryKey = "container_beta_custom_endpoint"
var ContainerBetaCustomEndpointEntry = &schema.Schema{
	Type:         schema.TypeString,
	Optional:     true,
	ValidateFunc: validateCustomEndpoint,
	DefaultFunc: schema.MultiEnvDefaultFunc([]string{
		"GOOGLE_CONTAINER_BETA_CUSTOM_ENDPOINT",
	}, ContainerBetaDefaultBasePath),
}

func validateCustomEndpoint(v interface{}, k string) (ws []string, errors []error) {
	re := `.*/[^/]+/$`
	return validateRegexp(re)(v, k)
}
