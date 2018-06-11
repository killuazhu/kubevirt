/*
 * This file is part of the KubeVirt project
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Copyright 2018 Red Hat, Inc.
 *
 */

package device_manager

import (
	"fmt"
	"os"

	"k8s.io/client-go/tools/cache"

	"github.com/fsnotify/fsnotify"

	"kubevirt.io/kubevirt/pkg/kubecli"
	"kubevirt.io/kubevirt/pkg/log"
)

const (
	KVMPath = "/dev/kvm"
	KVMName = "kvm"
	TunPath = "/dev/net/tun"
	TunName = "tun"
)

type DeviceController struct {
	clientset     kubecli.KubevirtClient
	devicePlugins []*GenericDevicePlugin
	host          string
	vmInformer    cache.SharedIndexInformer
}

func NewDeviceController(vmInformer cache.SharedIndexInformer, clientset kubecli.KubevirtClient, host string) *DeviceController {
	return &DeviceController{
		clientset: clientset,
		devicePlugins: []*GenericDevicePlugin{
			NewGenericDevicePlugin(KVMName, KVMPath),
			NewGenericDevicePlugin(TunName, TunPath),
		},
		host:       host,
		vmInformer: vmInformer,
	}
}

func (c *DeviceController) nodeHasDevice(devicePath string) bool {
	_, err := os.Stat(devicePath)
	// Since this is a boolean question, any error means "no"
	return (err == nil)
}

func (c *DeviceController) waitForPath(path string, stop chan struct{}) error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil
	}
	defer watcher.Close()

	watcher.Add(path)

	for {
		select {
		case event := <-watcher.Events:
			if event.Op == fsnotify.Create {
				return nil
			}
		case <-stop:
			return fmt.Errorf("shutting down")
		}
	}
}

func (c *DeviceController) StartDevicePlugin(dev *GenericDevicePlugin, stop chan struct{}) error {
	logger := log.DefaultLogger()
	if !c.nodeHasDevice(dev.devicePath) {
		logger.Infof("%s device not found. Waiting.", dev.deviceName)
		err := c.waitForPath(dev.devicePath, stop)
		if err != nil {
			logger.Errorf("error waiting for %s device: %v", dev.deviceName, err)
			return err
		}
	}

	err := dev.Start(stop)
	if err != nil {
		logger.Errorf("Error starting %s device plugin: %v", dev.deviceName, err)
		return err
	}
	return nil
}

func (c *DeviceController) Run(stop chan struct{}) error {
	logger := log.DefaultLogger()
	logger.Info("Starting device plugin controller")

	for _, dev := range c.devicePlugins {
		c.StartDevicePlugin(dev, stop)
	}

	<-stop

	logger.Info("Shutting down device plugin controller")
	return nil
}
