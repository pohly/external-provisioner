/*
Copyright 2017 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package csicommon

import (
	"fmt"
	"os"
	"strings"

	"github.com/container-storage-interface/spec/lib/go/csi/v0"
	"github.com/golang/glog"
	"golang.org/x/net/context"
	"google.golang.org/grpc"

	"github.com/opentracing/opentracing-go"
	otlog "github.com/opentracing/opentracing-go/log"
	jaegercfg "github.com/uber/jaeger-client-go/config"
)

func ParseEndpoint(ep string) (string, string, error) {
	if strings.HasPrefix(strings.ToLower(ep), "unix://") || strings.HasPrefix(strings.ToLower(ep), "tcp://") {
		s := strings.SplitN(ep, "://", 2)
		if s[1] != "" {
			return s[0], s[1], nil
		}
	}
	return "", "", fmt.Errorf("Invalid endpoint: %v", ep)
}

func NewVolumeCapabilityAccessMode(mode csi.VolumeCapability_AccessMode_Mode) *csi.VolumeCapability_AccessMode {
	return &csi.VolumeCapability_AccessMode{Mode: mode}
}

func NewDefaultNodeServer(d *CSIDriver) *DefaultNodeServer {
	return &DefaultNodeServer{
		Driver: d,
	}
}

func NewDefaultIdentityServer(d *CSIDriver) *DefaultIdentityServer {
	return &DefaultIdentityServer{
		Driver: d,
	}
}

func NewDefaultControllerServer(d *CSIDriver) *DefaultControllerServer {
	return &DefaultControllerServer{
		Driver: d,
	}
}

func NewControllerServiceCapability(cap csi.ControllerServiceCapability_RPC_Type) *csi.ControllerServiceCapability {
	return &csi.ControllerServiceCapability{
		Type: &csi.ControllerServiceCapability_Rpc{
			Rpc: &csi.ControllerServiceCapability_RPC{
				Type: cap,
			},
		},
	}
}

func RunNodePublishServer(endpoint string, d *CSIDriver, ns csi.NodeServer) {
	ids := NewDefaultIdentityServer(d)

	s := NewNonBlockingGRPCServer()
	s.Start(endpoint, ids, nil, ns)
	s.Wait()
}

func RunControllerPublishServer(endpoint string, d *CSIDriver, cs csi.ControllerServer) {
	ids := NewDefaultIdentityServer(d)

	s := NewNonBlockingGRPCServer()
	s.Start(endpoint, ids, cs, nil)
	s.Wait()
}

func RunControllerandNodePublishServer(endpoint string, d *CSIDriver, cs csi.ControllerServer, ns csi.NodeServer) {
	ids := NewDefaultIdentityServer(d)

	s := NewNonBlockingGRPCServer()
	s.Start(endpoint, ids, cs, ns)
	s.Wait()
}

// LogGRPCServer logs the server-side call information via glog.
//
// Warning: at log levels >= 5 the recorded information includes all
// parameters, which potentially contains sensitive information like
// the secrets.
func LogGRPCServer(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	glog.V(3).Infof("GRPC call: %s", info.FullMethod)
	glog.V(5).Infof("GRPC request: %+v", req)
	resp, err := handler(ctx, req)
	if err != nil {
		glog.Errorf("GRPC error: %v", err)
	} else {
		glog.V(5).Infof("GRPC response: %+v", resp)
	}
	return resp, err
}

// LogGRPCClient does the same as LogGRPCServer, only on the client side.
func LogGRPCClient(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
	glog.V(3).Infof("GRPC call: %s", method)
	glog.V(5).Infof("GRPC request: %+v", req)
	err := invoker(ctx, method, req, reply, cc, opts...)
	if err != nil {
		glog.Errorf("GRPC error: %v", err)
	} else {
		glog.V(5).Infof("GRPC response: %+v", reply)
	}
	return err
}

// TraceGRPCPayload adds the request and response as tags
// to the call's span, if the log level is five or higher.
// Warning: this may include sensitive information like the
// secrets.
func TraceGRPCPayload(sp opentracing.Span, method string, req, reply interface{}, err error) {
	if glog.V(5) {
		sp.SetTag("request", req)
		if err == nil {
			sp.SetTag("response", reply)
		}
	}
}

// Infof logs with glog.V(level).Infof() and in addition, always adds
// a log message to the current tracing span if the context has
// one. This ensures that spans which get recorded (not all do) have
// the full information.
func Infof(level glog.Level, ctx context.Context, format string, args ...interface{}) {
	glog.V(level).Infof(format, args...)
	sp := opentracing.SpanFromContext(ctx)
	if sp != nil {
		sp.LogFields(otlog.Lazy(func(fv otlog.Encoder) {
			fv.EmitString("message", fmt.Sprintf(format, args...))
		}))
	}
}

// Errorf does the same as Infof for error messages, except that
// it ignores the current log level.
func Errorf(ctx context.Context, format string, args ...interface{}) {
	glog.Errorf(format, args...)
	sp := opentracing.SpanFromContext(ctx)
	if sp != nil {
		sp.LogFields(otlog.Lazy(func(fv otlog.Encoder) {
			fv.EmitString("event", "error")
			fv.EmitString("message", fmt.Sprintf(format, args...))
		}))
	}
}

// InitTracer initializes the global OpenTracing tracer, using Jaeger
// and the provided name for the current process. Must be called at
// the start of main(). The result is a function which should be
// called at the end of main() to clean up.
func InitTracer(component string) func() {
	// Add support for the usual env variables, in particular
	// JAEGER_AGENT_HOST, which is needed when running only one
	// Jaeger agent per cluster.
	cfg, err := jaegercfg.FromEnv()
	if err != nil {
		// parsing errors might happen here, such as when we get a string where we expect a number
		glog.Errorf("Could not parse Jaeger env vars: %s", err.Error())
		os.Exit(1)
	}

	// Initialize tracer singleton.
	closer, err := cfg.InitGlobalTracer(component)
	if err != nil {
		glog.Errorf("Could not initialize Jaeger tracer: %s", err.Error())
		os.Exit(1)
	}
	return func() {
		closer.Close()
	}
}
