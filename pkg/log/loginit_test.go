// Copyright (C) 2020, 2022, Oracle and/or its affiliates.
// Licensed under the Universal Permissive License v 1.0 as shown at https://oss.oracle.com/licenses/upl.

package log

import (
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	kzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	"testing"
)

func TestInitLogsDefaultInfo(t *testing.T) {
	InitLogs(kzap.Options{})
	zap.S().Errorf("greeting %v", "hello")
	zap.S().Infof("greeting %v", "hello")
	zap.S().Debugf("greeting %v", "hello")
	msg := "InfoLevel is enabled"
	assert.NotNil(t, zap.L().Check(zapcore.InfoLevel, msg), msg)
	msg = "ErrorLevel is enabled"
	assert.NotNil(t, zap.L().Check(zapcore.ErrorLevel, msg), msg)
	msg = "DebugLevel is disabled"
	assert.Nil(t, zap.L().Check(zapcore.DebugLevel, msg), msg)
}

func TestInitLogsNonDefaultInfo(t *testing.T) {
	testOpts := kzap.Options{}
	testOpts.Development = true
	testOpts.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
	InitLogs(testOpts)
	zap.S().Errorf("greeting %v", "hello")
	zap.S().Infof("greeting %v", "hello")
	zap.S().Debugf("greeting %v", "hello")
	msg := "InfoLevel is enabled"
	assert.NotNil(t, zap.L().Check(zapcore.InfoLevel, msg), msg)
	msg = "ErrorLevel is enabled"
	assert.NotNil(t, zap.L().Check(zapcore.ErrorLevel, msg), msg)
	msg = "DebugLevel is disabled"
	assert.Nil(t, zap.L().Check(zapcore.DebugLevel, msg), msg)
}