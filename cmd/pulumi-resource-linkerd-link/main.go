// Copyright 2016-2020, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"

	pbempty "github.com/golang/protobuf/ptypes/empty"
	multiclustercmd "github.com/linkerd/linkerd2/multicluster/cmd"
	"github.com/pulumi/pulumi/pkg/v3/resource/provider"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/cmdutil"
	rpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
)

// Injected by linker in release builds.
var version string

func main() {
	err := provider.Main("linkerd-link", func(host *provider.HostClient) (rpc.ResourceProviderServer, error) {
		return &linkerdLinkProvider{
			host: host,
		}, nil
	})
	if err != nil {
		cmdutil.ExitError(err.Error())
	}
}

type linkerdLinkProvider struct {
	host *provider.HostClient
}

func (k *linkerdLinkProvider) CheckConfig(ctx context.Context, req *rpc.CheckRequest) (*rpc.CheckResponse, error) {
	return &rpc.CheckResponse{Inputs: req.GetNews()}, nil
}

func (k *linkerdLinkProvider) DiffConfig(ctx context.Context, req *rpc.DiffRequest) (*rpc.DiffResponse, error) {
	return &rpc.DiffResponse{}, nil
}

func (k *linkerdLinkProvider) Configure(ctx context.Context, req *rpc.ConfigureRequest) (*rpc.ConfigureResponse, error) {
	return &rpc.ConfigureResponse{}, nil
}

func (k *linkerdLinkProvider) Invoke(_ context.Context, req *rpc.InvokeRequest) (*rpc.InvokeResponse, error) {
	tok := req.GetTok()
	return nil, fmt.Errorf("Unknown Invoke token '%s'", tok)
}

func (k *linkerdLinkProvider) StreamInvoke(req *rpc.InvokeRequest, server rpc.ResourceProvider_StreamInvokeServer) error {
	tok := req.GetTok()
	return fmt.Errorf("Unknown StreamInvoke token '%s'", tok)
}

func (k *linkerdLinkProvider) Check(ctx context.Context, req *rpc.CheckRequest) (*rpc.CheckResponse, error) {
	urn := resource.URN(req.GetUrn())
	ty := urn.Type()
	if ty != "linkerd-link:index:Link" {
		return nil, fmt.Errorf("Unknown resource type '%s'", ty)
	}
	return &rpc.CheckResponse{Inputs: req.News, Failures: nil}, nil
}

func (k *linkerdLinkProvider) Diff(ctx context.Context, req *rpc.DiffRequest) (*rpc.DiffResponse, error) {
	urn := resource.URN(req.GetUrn())
	ty := urn.Type()
	if ty != "linkerd-link:index:Link" {
		return nil, fmt.Errorf("Unknown resource type '%s'", ty)
	}

	// TODO: Do work here.

	return &rpc.DiffResponse{}, nil
}

func (k *linkerdLinkProvider) Create(ctx context.Context, req *rpc.CreateRequest) (*rpc.CreateResponse, error) {
	urn := resource.URN(req.GetUrn())
	ty := urn.Type()
	if ty != "linkerd-link:index:Link" {
		return nil, fmt.Errorf("Unknown resource type '%s'", ty)
	}

	props := req.GetProperties()
	inputs, err := plugin.UnmarshalProperties(props, plugin.MarshalOptions{KeepUnknowns: true, SkipNulls: true})

	config, err := runMulticlusterLink([]string{
		"--context",
		inputs["from_cluster_kubeconfig"].StringValue(),
		"link",
		"--cluster-name",
		inputs["from_cluster_name"].StringValue(),
	})
	if err != nil {
		return nil, err
	}
	plugin.MarshalProperties(
		resource.NewPropertyMapFromMap(map[string]interface{}{"config_group_yaml": config}),
		plugin.MarshalOptions{KeepUnknowns: true, SkipNulls: true},
	)

	return &rpc.CreateResponse{}, nil
}

func (k *linkerdLinkProvider) Read(ctx context.Context, req *rpc.ReadRequest) (*rpc.ReadResponse, error) {
	urn := resource.URN(req.GetUrn())
	ty := urn.Type()
	if ty != "linkerd-link:index:Link" {
		return nil, fmt.Errorf("Unknown resource type '%s'", ty)
	}
	return &rpc.ReadResponse{
		Id:         req.GetId(),
		Properties: req.GetProperties(),
	}, nil
}

func (k *linkerdLinkProvider) Update(ctx context.Context, req *rpc.UpdateRequest) (*rpc.UpdateResponse, error) {
	urn := resource.URN(req.GetUrn())
	ty := urn.Type()
	if ty != "linkerd-link:index:Link" {
		return nil, fmt.Errorf("Unknown resource type '%s'", ty)
	}

	// TODO: do work here.

	return &rpc.UpdateResponse{}, nil
}

func (k *linkerdLinkProvider) Delete(ctx context.Context, req *rpc.DeleteRequest) (*pbempty.Empty, error) {
	urn := resource.URN(req.GetUrn())
	ty := urn.Type()
	if ty != "linkerd-link:index:Link" {
		return nil, fmt.Errorf("Unknown resource type '%s'", ty)
	}

	// TODO: do work here? (I don't think we need to.)

	return &pbempty.Empty{}, nil
}

func (k *linkerdLinkProvider) Construct(_ context.Context, _ *rpc.ConstructRequest) (*rpc.ConstructResponse, error) {
	panic("Construct not implemented")
}

func (k *linkerdLinkProvider) GetPluginInfo(context.Context, *pbempty.Empty) (*rpc.PluginInfo, error) {
	return &rpc.PluginInfo{
		Version: version,
	}, nil
}

func (k *linkerdLinkProvider) GetSchema(ctx context.Context, req *rpc.GetSchemaRequest) (*rpc.GetSchemaResponse, error) {
	return &rpc.GetSchemaResponse{}, nil
}

func (k *linkerdLinkProvider) Cancel(context.Context, *pbempty.Empty) (*pbempty.Empty, error) {
	return &pbempty.Empty{}, nil
}

func runMulticlusterLink(args []string) (string, error) {
	cmd := multiclustercmd.NewCmdMulticluster()
	cmd.SetArgs(args)
	b := bytes.NewBufferString("")
	cmd.SetOut(b)
	err := cmd.Execute()
	if err != nil {
		return "", err
	}
	out, err := ioutil.ReadAll(b)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
