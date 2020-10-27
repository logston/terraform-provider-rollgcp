package rolleksgcp

import (
	"context"
	"fmt"
	"reflect"
	"regexp"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

var (
	instanceGroupManagerURL = regexp.MustCompile(fmt.Sprintf("projects/(%s)/zones/([a-z0-9-]*)/instanceGroupManagers/([^/]*)", ProjectRegex))
)

// Setting a guest accelerator block to count=0 is the equivalent to omitting the block: it won't get
// sent to the API and it won't be stored in state. This diffFunc will try to compare the old + new state
// by only comparing the blocks with a positive count and ignoring those with count=0
//
// One quirk with this approach is that configs with mixed count=0 and count>0 accelerator blocks will
// show a confusing diff if one of there are config changes that result in a legitimate diff as the count=0
// blocks will not be in state.
//
// This could also be modelled by setting `guest_accelerator = []` in the config. However since the
// previous syntax requires that schema.SchemaConfigModeAttr is set on the field it is advisable that
// we have a work around for removing guest accelerators. Also Terraform 0.11 cannot use dynamic blocks
// so this isn't a solution for module authors who want to dynamically omit guest accelerators
// See https://github.com/hashicorp/terraform-provider-google/issues/3786
func resourceNodeConfigEmptyGuestAccelerator(_ context.Context, diff *schema.ResourceDiff, meta interface{}) error {
	old, new := diff.GetChange("node_config.0.guest_accelerator")
	oList := old.([]interface{})
	nList := new.([]interface{})

	if len(nList) == len(oList) || len(nList) == 0 {
		return nil
	}
	var hasAcceleratorWithEmptyCount bool
	// the list of blocks in a desired state with count=0 accelerator blocks in it
	// will be longer than the current state.
	// this index tracks the location of positive count accelerator blocks
	index := 0
	for i, item := range nList {
		accel := item.(map[string]interface{})
		if accel["count"].(int) == 0 {
			hasAcceleratorWithEmptyCount = true
			// Ignore any 'empty' accelerators because they aren't sent to the API
			continue
		}
		if index >= len(oList) {
			// Return early if there are more positive count accelerator blocks in the desired state
			// than the current state since a difference in 'legit' blocks is a valid diff.
			// This will prevent array index overruns
			return nil
		}
		if !reflect.DeepEqual(nList[i], oList[index]) {
			return nil
		}
		index += 1
	}

	if hasAcceleratorWithEmptyCount && index == len(oList) {
		// If the number of count>0 blocks match, there are count=0 blocks present and we
		// haven't already returned due to a legitimate diff
		err := diff.Clear("node_config.0.guest_accelerator")
		if err != nil {
			return err
		}
	}

	return nil
}

func containerClusterMutexKey(project, location, clusterName string) string {
	return fmt.Sprintf("google-container-cluster/%s/%s/%s", project, location, clusterName)
}

// Suppress unremovable default scope values from GCP.
// If the default service account would not otherwise have it, the `monitoring.write` scope
// is added to a GKE cluster's scopes regardless of what the user provided.
// monitoring.write is inherited from monitoring (rw) and cloud-platform, so it won't always
// be present.
// Enabling Stackdriver features through logging_service and monitoring_service may enable
// monitoring or logging.write. We've chosen not to suppress in those cases because they're
// removable by disabling those features.
func containerClusterAddedScopesSuppress(k, old, new string, d *schema.ResourceData) bool {
	o, n := d.GetChange("cluster_autoscaling.0.auto_provisioning_defaults.0.oauth_scopes")
	if o == nil || n == nil {
		return false
	}

	addedScopes := []string{
		"https://www.googleapis.com/auth/monitoring.write",
	}

	// combine what the default scopes are with what was passed
	m := golangSetFromStringSlice(append(addedScopes, convertStringArr(n.([]interface{}))...))
	combined := stringSliceFromGolangSet(m)

	// compare if the combined new scopes and default scopes differ from the old scopes
	if len(combined) != len(convertStringArr(o.([]interface{}))) {
		return false
	}

	for _, i := range combined {
		if stringInSlice(convertStringArr(o.([]interface{})), i) {
			continue
		}

		return false
	}

	return true
}
