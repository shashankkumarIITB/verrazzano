// Copyright (c) 2021, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package appoper

import (
	"fmt"
	"github.com/verrazzano/verrazzano/pkg/bom"
	"github.com/verrazzano/verrazzano/platform-operator/constants"
	"github.com/verrazzano/verrazzano/platform-operator/controllers/verrazzano/component/spi"
	"github.com/verrazzano/verrazzano/platform-operator/internal/k8s/status"
	"k8s.io/apimachinery/pkg/types"
	"os"
)

// ComponentName is the name of the component
const ComponentName = "verrazzano-application-operator"

// AppendApplicationOperatorOverrides Honor the APP_OPERATOR_IMAGE env var if set; this allows an explicit override
// of the verrazzano-application-operator image when set.
func AppendApplicationOperatorOverrides(_ spi.ComponentContext, _ string, _ string, _ string, kvs []bom.KeyValue) ([]bom.KeyValue, error) {
	envImageOverride := os.Getenv(constants.VerrazzanoAppOperatorImageEnvVar)
	if len(envImageOverride) == 0 {
		return kvs, nil
	}
	kvs = append(kvs, bom.KeyValue{
		Key:   "image",
		Value: envImageOverride,
	})
	fmt.Println("Foo")
	return kvs, nil
}

// IsApplicationOperatorReady checks if the application operator deployment is ready
func IsApplicationOperatorReady(ctx spi.ComponentContext, name string, namespace string) bool {
	deployments := []types.NamespacedName{
		{Name: "verrazzano-application-operator", Namespace: namespace},
	}
	return status.DeploymentsReady(ctx.Log(), ctx.Client(), deployments, 1)
}