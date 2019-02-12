/*
Copyright 2018 The Service Fabrik Authors.

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

package factory

import (
	stdlog "log"
	"os"
	"testing"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var cfg *rest.Config
var c client.Client
var mgr manager.Manager

func TestMain(m *testing.M) {
	var err error
	t := &envtest.Environment{}
	if cfg, err = t.Start(); err != nil {
		stdlog.Fatal(err)
	}

	if c, err = client.New(cfg, client.Options{Scheme: scheme.Scheme}); err != nil {
		stdlog.Fatal(err)
	}

	if mgr, err = manager.New(cfg, manager.Options{}); err != nil {
		stdlog.Fatal(err)
	}

	code := m.Run()
	t.Stop()
	os.Exit(code)
}
