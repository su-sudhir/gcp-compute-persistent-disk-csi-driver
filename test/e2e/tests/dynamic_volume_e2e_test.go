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

// Volume Lifecycle (Create and Delete)
//
// Verifies the full lifecycle of a dynamic volume:
//   - CreateVolume resolves "dynamic" to the correct disk type (hyperdisk-balanced on HD node)
//   - The GCE disk is created in READY state with a valid volume ID
//   - DeleteVolume removes the disk completely with no orphaned resources
//
// Test flow:
//  1. CreateVolume with type=dynamic on an HD-capable node (c3-standard-4)
//  2. Verify GCE disk exists, is READY, and has correct type (hyperdisk-balanced)
//  3. DeleteVolume via CSI RPC
//  4. Verify GCE disk is fully gone (notFound) — no orphaned resources
var _ = Describe("GCE PD CSI Driver Dynamic Volumes Lifecycle", func() {

	It("Should create a dynamic volume with correct disk type and delete it with no orphaned resources", func() {
		// getRandomMwTestContext() returns a c3-standard-4 VM which supports hyperdisk
		testContext := getRandomMwTestContext()

		p, z, _ := testContext.Instance.GetIdentity()
		client := testContext.Client

		volName := testNamePrefix + string(uuid.NewUUID())

		params := map[string]string{
			parameters.ParameterKeyType: parameters.DynamicVolumeType,
			parameters.ParameterHDType:  "hyperdisk-balanced",
			parameters.ParameterPDType:  "pd-balanced",
		}

		// Step 1: Create volume via CSI RPC
		volume, err := client.CreateVolume(volName, params, defaultHdBSizeGb,
			&csi.TopologyRequirement{
				Requisite: []*csi.Topology{
					{
						Segments: map[string]string{
							"topology.gke.io/zone": z,
						},
					},
				},
				Preferred: []*csi.Topology{
					{
						Segments: map[string]string{
							"topology.gke.io/zone":                        z,
							common.DiskTypeLabelKey("hyperdisk-balanced"): "true",
							common.DiskTypeLabelKey("pd-balanced"):        "true",
						},
					},
				},
			}, nil)
		Expect(err).To(BeNil(), "CreateVolume failed: %v", err)

		// Step 2: Verify volume ID is valid and GCE disk exists in READY state
		Expect(volume.VolumeId).NotTo(BeEmpty(), "VolumeId should not be empty")

		project, key, err := common.VolumeIDToKey(volume.VolumeId)
		Expect(err).To(BeNil(), "Failed to parse volume ID: %v", err)

		cloudDisk, err := computeService.Disks.Get(p, z, volName).Do()
		Expect(err).To(BeNil(), "GCE disk not found after CreateVolume: %v", err)
		Expect(cloudDisk.Status).To(Equal(readyState), "Disk not in READY state")
		Expect(cloudDisk.Name).To(Equal(volName), "Disk name mismatch")

		klog.Infof("Dynamic volume created: name=%s type=%s zone=%s", cloudDisk.Name, cloudDisk.Type, z)
		Expect(cloudDisk.Type).To(ContainSubstring("hyperdisk-balanced"),
			"Expected hyperdisk-balanced but got: %s", cloudDisk.Type)

		// Step 3: Delete volume via CSI RPC
		err = client.DeleteVolume(volume.VolumeId)
		Expect(err).To(BeNil(), "DeleteVolume failed: %v", err)

		// Step 4: Verify disk is fully gone — no orphaned resources
		_, err = computeService.Disks.Get(project, key.Zone, key.Name).Do()
		Expect(gce.IsGCEError(err, "notFound")).To(BeTrue(),
			"Expected disk to be fully deleted but it still exists")

		klog.Infof("Dynamic volume deleted successfully: name=%s — no orphaned resources", volName)
	})

})
