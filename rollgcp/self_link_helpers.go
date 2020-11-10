package rollgcp

import (
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	glBeta "github.com/hashicorp/terraform-provider-google-beta/google-beta"
)

// Compare only the relative path of two self links.
func compareSelfLinkRelativePaths(_, old, new string, _ *schema.ResourceData) bool {
	oldStripped, err := getRelativePath(old)
	if err != nil {
		return false
	}

	newStripped, err := getRelativePath(new)
	if err != nil {
		return false
	}

	if oldStripped == newStripped {
		return true
	}

	return false
}

// compareSelfLinkOrResourceName checks if two resources are the same resource
//
// Use this method when the field accepts either a name or a self_link referencing a resource.
// The value we store (i.e. `old` in this method), must be a self_link.
func compareSelfLinkOrResourceName(_, old, new string, _ *schema.ResourceData) bool {
	newParts := strings.Split(new, "/")

	if len(newParts) == 1 {
		// `new` is a name
		// `old` is always a self_link
		if glBeta.GetResourceNameFromSelfLink(old) == newParts[0] {
			return true
		}
	}

	// The `new` string is a self_link
	return compareSelfLinkRelativePaths("", old, new, nil)
}

func getRelativePath(selfLink string) (string, error) {
	stringParts := strings.SplitAfterN(selfLink, "projects/", 2)
	if len(stringParts) != 2 {
		return "", fmt.Errorf("String was not a self link: %s", selfLink)
	}

	return "projects/" + stringParts[1], nil
}
