// Copyright (c) 2022, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package ocidns

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func OCIDNS(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Update OCI DNS Suite")
}
