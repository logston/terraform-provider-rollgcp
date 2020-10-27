package rolleksgcp

import (
	"fmt"
)

func getProjectFromSchema(projectSchemaField string, d TerraformResourceData, config *Config) (string, error) {
	res, ok := d.GetOk(projectSchemaField)
	if ok && projectSchemaField != "" {
		return res.(string), nil
	}
	if config.Project != "" {
		return config.Project, nil
	}
	return "", fmt.Errorf("%s: required field is not set", projectSchemaField)
}
