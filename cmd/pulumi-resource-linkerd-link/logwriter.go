package main

import (
	"context"

	"github.com/pulumi/pulumi/pkg/v3/resource/provider"
	"github.com/pulumi/pulumi/sdk/v3/go/common/diag"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
)

// Stolen from pulumi-docker-buildkit.
type logWriter struct {
	ctx      context.Context
	host     *provider.HostClient
	urn      resource.URN
	severity diag.Severity
}

func (w *logWriter) Write(p []byte) (n int, err error) {
	return len(p), w.host.Log(w.ctx, w.severity, w.urn, string(p))
}
