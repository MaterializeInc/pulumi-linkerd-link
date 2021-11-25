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
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"

	pbempty "github.com/golang/protobuf/ptypes/empty"
	multiclustercmd "github.com/linkerd/linkerd2/multicluster/cmd"
	"github.com/pulumi/pulumi/pkg/v3/resource/provider"
	"github.com/pulumi/pulumi/sdk/v3/go/common/diag"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/cmdutil"
	rpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"google.golang.org/protobuf/types/known/structpb"
)

// Injected by linker in release builds.
var version string

var linkerdInvocationArg = "--internal-only-invoke-linkerd-cli"

func main() {
	// Cursed code alert: Since linkerd's only public interface
	// for creating a multicluster link is currently to run the
	// CLI, and we don't want to require users to install the
	// linkerd binary (at least I don't), and linkerd doesn't
	// allow overriding its Stdout, here's what we do:
	//
	// When this program is invoked with
	// --internal-only-invoke-linkerd-cli, it acts as a "linkerd
	// multicluster" binary. Otherwise, it is a real pulumi
	// provider.
	//
	// TODO: Use real data structures when https://github.com/linkerd/linkerd2/pull/7335/files lands.
	if len(os.Args) > 1 && os.Args[1] == linkerdInvocationArg {
		if err := runMulticlusterAsChild(os.Args[2:]); err != nil {
			log.Fatal(err)
		}
		return
	}
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

	olds, err := plugin.UnmarshalProperties(req.GetOlds(), plugin.MarshalOptions{KeepUnknowns: true, SkipNulls: true})
	if err != nil {
		return nil, err
	}
	oldKubecfg, err := normalizeKubecfg(olds["from_cluster_kubeconfig"])
	if err != nil {
		return nil, fmt.Errorf("old from_cluster_kubeconfig is invalid: %v", err)
	}
	oldToKubecfg, err := normalizeKubecfg(olds["to_cluster_kubeconfig"])
	if err != nil {
		return nil, fmt.Errorf("old to_cluster_kubeconfig is invalid: %v", err)
	}

	news, err := plugin.UnmarshalProperties(req.GetNews(), plugin.MarshalOptions{KeepUnknowns: true, SkipNulls: true})
	if err != nil {
		return nil, err
	}
	newKubecfg, err := normalizeKubecfg(news["from_cluster_kubeconfig"])
	if err != nil {
		return nil, fmt.Errorf("new from_cluster_kubeconfig is invalid: %v", err)
	}
	newToKubecfg, err := normalizeKubecfg(news["to_cluster_kubeconfig"])
	if err != nil {
		return nil, fmt.Errorf("new to_cluster_kubeconfig is invalid: %v", err)
	}
	if bytes.Compare(oldKubecfg, newKubecfg) == 0 {
		delete(olds, "from_cluster_kubeconfig")
		delete(news, "from_cluster_kubeconfig")
	}
	if bytes.Compare(oldToKubecfg, newToKubecfg) == 0 {
		delete(olds, "to_cluster_kubeconfig")
		delete(news, "to_cluster_kubeconfig")
	}

	d := olds.Diff(news)
	if d == nil {
		return &rpc.DiffResponse{
			Changes: rpc.DiffResponse_DIFF_NONE,
		}, nil
	}

	diff := map[string]*rpc.PropertyDiff{}
	for key := range d.Adds {
		diff[string(key)] = &rpc.PropertyDiff{Kind: rpc.PropertyDiff_ADD}
	}
	for key := range d.Deletes {
		diff[string(key)] = &rpc.PropertyDiff{Kind: rpc.PropertyDiff_DELETE}
	}
	for key := range d.Updates {
		diff[string(key)] = &rpc.PropertyDiff{Kind: rpc.PropertyDiff_UPDATE}
	}
	return &rpc.DiffResponse{
		Changes:         rpc.DiffResponse_DIFF_SOME,
		DetailedDiff:    diff,
		HasDetailedDiff: true,
	}, nil
}

func (k *linkerdLinkProvider) Create(ctx context.Context, req *rpc.CreateRequest) (*rpc.CreateResponse, error) {
	urn := resource.URN(req.GetUrn())
	ty := urn.Type()
	if ty != "linkerd-link:index:Link" {
		return nil, fmt.Errorf("Unknown resource type '%s'", ty)
	}

	props := req.GetProperties()
	outputProperties, err := k.linkOtherCluster(ctx, urn, props)
	if err != nil {
		return nil, err
	}
	return &rpc.CreateResponse{
		Id:         "ignored",
		Properties: outputProperties,
	}, nil
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
	old := req.GetOlds()
	err := k.unlinkOtherCluster(ctx, urn, old)
	if err != nil {
		return nil, fmt.Errorf("could not remove old resources: %v", err)
	}

	props := req.GetNews()
	outputProperties, err := k.linkOtherCluster(ctx, urn, props)
	if err != nil {
		return nil, err
	}
	return &rpc.UpdateResponse{
		Properties: outputProperties,
	}, nil
}

func (k *linkerdLinkProvider) Delete(ctx context.Context, req *rpc.DeleteRequest) (*pbempty.Empty, error) {
	urn := resource.URN(req.GetUrn())
	ty := urn.Type()
	if ty != "linkerd-link:index:Link" {
		return nil, fmt.Errorf("Unknown resource type '%s'", ty)
	}

	props := req.GetProperties()
	return &pbempty.Empty{}, k.unlinkOtherCluster(ctx, urn, props)
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

// runMulticluster runs this provider as a program that emulates
// "linkerd multicluster", reading its output and reporting it as an
// output property.
func (k *linkerdLinkProvider) runMulticluster(ctx context.Context, urn resource.URN, args []string) (string, error) {
	a := []string{linkerdInvocationArg}
	a = append(a, args...)
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("could not determine currently running binary: %v", err)
	}
	cmd := exec.Command(executable, a...)
	var stdout bytes.Buffer
	cmd.Stdin = &bytes.Buffer{}
	cmd.Stdout = &stdout
	cmd.Stderr = &logWriter{ctx, k.host, urn, diag.Warning}

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("could not run self: %v", err)
	}
	return string(stdout.Bytes()), nil
}

func runMulticlusterAsChild(args []string) error {
	cmd := multiclustercmd.NewCmdMulticluster()
	cmd.SetArgs(args)
	cmd.SetOut(os.Stderr)
	return cmd.Execute()
}

func normalizeKubecfg(raw resource.PropertyValue) ([]byte, error) {
	if !raw.IsString() && !raw.IsObject() {
		return nil, fmt.Errorf("kubeconfig must be either a structure or a string, got: %v", raw.TypeString())
	}

	var config map[string]interface{}
	if raw.IsString() {
		// kubeconfig is documented to be either a string
		// containing json-serialized data, or a file path: We
		// forbid file paths.
		err := json.Unmarshal([]byte(raw.StringValue()), &config)
		if err != nil {
			return nil, fmt.Errorf("could not deserialize string-typed kubeconfig: %v", err)
		}
	} else if raw.IsObject() {
		config = raw.ObjectValue().Mappable()
	}
	val, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("could not marshal object-typed kubecfg: %v", err)
	}
	return val, nil
}

func writeKubeConfig(kubeconfigStr []byte) (*os.File, error) {
	f, err := os.CreateTemp("", "kubeconfig")
	if err != nil {
		return nil, fmt.Errorf("opening temporary kubeconfig: %v", err)
	}
	f.Write(kubeconfigStr)
	err = f.Close()
	if err != nil {
		return nil, fmt.Errorf("closing temp file: %v", err)
	}
	return f, nil
}

func (k *linkerdLinkProvider) linkOtherCluster(ctx context.Context, urn resource.URN, props *structpb.Struct) (*structpb.Struct, error) {
	inputs, err := plugin.UnmarshalProperties(props, plugin.MarshalOptions{KeepUnknowns: true, SkipNulls: true})

	kubeconfigStr, err := normalizeKubecfg(inputs["from_cluster_kubeconfig"])
	if err != nil {
		return nil, fmt.Errorf("couldn't normalize kubeconfig: %v", err)
	}
	f, err := writeKubeConfig(kubeconfigStr)
	defer os.Remove(f.Name())

	clusterName := inputs["from_cluster_name"].StringValue()
	linkerdVersion := inputs["control_plane_image_version"].StringValue()
	_, err = k.runMulticluster(ctx, urn, []string{
		"--kubeconfig",
		f.Name(),
		"link",
		"--cluster-name",
		clusterName,
		"--control-plane-version",
		linkerdVersion,
	})
	if err != nil {
		return nil, fmt.Errorf("creating link kubernetes config: %v", err)
	}

	toKubeconfigStr, err := normalizeKubecfg(inputs["to_cluster_kubeconfig"])
	if err != nil {
		return nil, fmt.Errorf("couldn't normalize kubeconfig: %v", err)
	}
	to, err := writeKubeConfig(toKubeconfigStr)
	defer os.Remove(to.Name())

	kc := exec.Command("kubectl", "--kubeconfig", to.Name(), "apply", "-f", "-")
	kc.Stdin = &bytes.Buffer{}
	kc.Stdout = &logWriter{ctx, k.host, urn, diag.Info}
	kc.Stderr = &logWriter{ctx, k.host, urn, diag.Warning}
	err = kc.Run()
	if err != nil {
		return nil, fmt.Errorf("applying config with kubectl: %v", err)
	}
	return plugin.MarshalProperties(
		resource.NewPropertyMapFromMap(map[string]interface{}{
			"from_cluster_kubeconfig":     string(kubeconfigStr),
			"to_cluster_kubeconfig":       string(toKubeconfigStr),
			"from_cluster_name":           clusterName,
			"control_plane_image_version": linkerdVersion,
		}),
		plugin.MarshalOptions{KeepUnknowns: true, SkipNulls: true},
	)
}

// unlinkOtherCluster runs kubectl to remove outdated resource
// definitions for a multicluster link from a k8s cluster.
func (k *linkerdLinkProvider) unlinkOtherCluster(ctx context.Context, urn resource.URN, props *structpb.Struct) error {
	inputs, err := plugin.UnmarshalProperties(props, plugin.MarshalOptions{KeepUnknowns: true, SkipNulls: true})

	kubeconfigStr, err := normalizeKubecfg(inputs["to_cluster_kubeconfig"])
	if err != nil {
		return fmt.Errorf("couldn't normalize kubeconfig: %v", err)
	}
	f, err := writeKubeConfig(kubeconfigStr)
	defer os.Remove(f.Name())

	clusterName := inputs["from_cluster_name"].StringValue()
	config, err := k.runMulticluster(ctx, urn, []string{
		"--kubeconfig",
		f.Name(), // counter-intuitively, here we need the *destination* kubecfg.
		"unlink",
		"--cluster-name",
		clusterName,
	})
	if err != nil {
		return fmt.Errorf("creating link kubernetes config: %v", err)
	}

	kc := exec.Command("kubectl", "--kubeconfig", f.Name(), "delete", "-f", "-")
	kc.Stdin = bytes.NewBuffer([]byte(config))
	kc.Stdout = &logWriter{ctx, k.host, urn, diag.Info}
	kc.Stderr = &logWriter{ctx, k.host, urn, diag.Warning}
	err = kc.Run()
	if err != nil {
		return fmt.Errorf("removing config with kubectl: %v", err)
	}
	return nil

}
