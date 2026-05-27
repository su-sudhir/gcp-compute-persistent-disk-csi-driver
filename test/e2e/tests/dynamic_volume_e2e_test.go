/*
Copyright 2024 The Kubernetes Authors.

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

package tests

import (
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/klog/v2"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/common"
	gce "sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/gce-cloud-provider/compute"
	"sigs.k8s.io/gcp-compute-persistent-disk-csi-driver/pkg/parameters"

	csi "github.com/container-storage-interface/spec/lib/go/csi"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// Default Parameters Provisioning
//
// Verifies that when type=dynamic is used without specifying pd-type or hyperdisk-type,
// the driver uses the built-in defaults: hyperdisk-balanced on HD-capable nodes
// and pd-balanced on PD-only nodes.
//
// Test flow:
//  1. On HD-capable node (c3-standard-4): CreateVolume with only type=dynamic
//     → verify disk created as hyperdisk-balanced (default HD type)
//  2. On PD-only node (n2d-standard-4): CreateVolume with only type=dynamic
//     → verify disk created as pd-balanced (default PD type)
//  3. Cleanup: DeleteVolume for both and confirm disks are gone
var _ = Describe("GCE PD CSI Driver Dynamic Volumes Default Parameters Provisioning", func() {

	It("Should use built-in default disk types when no pd-type or hyperdisk-type is specified", func() {
		hdContext := getRandomMwTestContext()
		pdContext := getRandomTestContext()

		hdProject, hdZone, _ := hdContext.Instance.GetIdentity()
		pdProject, pdZone, _ := pdContext.Instance.GetIdentity()

		// Only type=dynamic — no pd-type or hyperdisk-type specified
		params := map[string]string{
			parameters.ParameterKeyType: parameters.DynamicVolumeType,
		}

		// --- HD-capable node: expect default hyperdisk-balanced ---
		hdVolName := testNamePrefix + string(uuid.NewUUID())
		hdVolume, err := hdContext.Client.CreateVolume(hdVolName, params, defaultHdBSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{Segments: map[string]string{"topology.gke.io/zone": hdZone}},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{
							"topology.gke.io/zone":                          hdZone,
							common.DiskTypeLabelKey("hyperdisk-balanced"):   "true",
							common.DiskTypeLabelKey("hyperdisk-throughput"): "true",
							common.DiskTypeLabelKey("pd-balanced"):          "true",
						},
					},
				},
			}, nil)
		Expect(err).To(BeNil(), "CreateVolume (HD) failed: %v", err)
		defer func() {
			err := hdContext.Client.DeleteVolume(hdVolume.VolumeId)
			Expect(err).To(BeNil(), "DeleteVolume (HD) failed")
			project, key, err := common.VolumeIDToKey(hdVolume.VolumeId)
			Expect(err).To(BeNil())
			_, err = computeService.Disks.Get(project, key.Zone, key.Name).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected HD disk to be deleted")
		}()

		hdDisk, err := computeService.Disks.Get(hdProject, hdZone, hdVolName).Do()
		Expect(err).To(BeNil(), "Could not get HD disk from GCE API")
		Expect(hdDisk.Status).To(Equal(readyState))
		klog.Infof("Default dynamic volume on HD node resolved to: %s", hdDisk.Type)
		Expect(hdDisk.Type).To(ContainSubstring("hyperdisk-balanced"),
			"Expected default hyperdisk-balanced on HD node but got: %s", hdDisk.Type)

		// --- PD-only node: expect default pd-balanced ---
		pdVolName := testNamePrefix + string(uuid.NewUUID())
		pdVolume, err := pdContext.Client.CreateVolume(pdVolName, params, defaultSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{Segments: map[string]string{"topology.gke.io/zone": pdZone}},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{
							"topology.gke.io/zone":                 pdZone,
							common.DiskTypeLabelKey("pd-balanced"): "true",
							common.DiskTypeLabelKey("pd-standard"): "true",
							common.DiskTypeLabelKey("pd-ssd"):      "true",
							common.DiskTypeLabelKey("pd-extreme"):  "true",
						},
					},
				},
			}, nil)
		Expect(err).To(BeNil(), "CreateVolume (PD) failed: %v", err)
		defer func() {
			err := pdContext.Client.DeleteVolume(pdVolume.VolumeId)
			Expect(err).To(BeNil(), "DeleteVolume (PD) failed")
			project, key, err := common.VolumeIDToKey(pdVolume.VolumeId)
			Expect(err).To(BeNil())
			_, err = computeService.Disks.Get(project, key.Zone, key.Name).Do()
			Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(), "Expected PD disk to be deleted")
		}()

		pdDisk, err := computeService.Disks.Get(pdProject, pdZone, pdVolName).Do()
		Expect(err).To(BeNil(), "Could not get PD disk from GCE API")
		Expect(pdDisk.Status).To(Equal(readyState))
		klog.Infof("Default dynamic volume on PD-only node resolved to: %s", pdDisk.Type)
		Expect(pdDisk.Type).To(ContainSubstring("pd-balanced"),
			"Expected default pd-balanced on PD-only node but got: %s", pdDisk.Type)
	})

})
