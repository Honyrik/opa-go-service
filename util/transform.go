package util

import (
	"github.com/fatih/structs"
	"github.com/open-policy-agent/opa/rego"
)

func ResultSetTArrayMap(result rego.ResultSet) []map[string]interface{} {
	if len(result) < 1 {
		return nil
	}
	var res []map[string]interface{}

	for _, elementResult := range result {
		res = append(res, structs.Map(elementResult))
	}

	return res
}
