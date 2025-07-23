/*
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

package expiration_test

import (
	"fmt"
	"github.com/samber/lo"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"testing"

	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"

	"github.com/zoom/karpenter-oci/pkg/apis/v1alpha1"
	"github.com/zoom/karpenter-oci/test/pkg/environment/oci"

	coretest "sigs.k8s.io/karpenter/pkg/test"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var env *oci.Environment
var nodeClass *v1alpha1.OciNodeClass
var nodePool *karpv1.NodePool

func TestExpiration(t *testing.T) {
	RegisterFailHandler(Fail)
	BeforeSuite(func() {
		env = oci.NewEnvironment(t)
	})
	AfterSuite(func() {
		env.Stop()
	})
	RunSpecs(t, "Expiration")
}

var _ = BeforeEach(func() {
	env.BeforeEach()
	nodeClass = env.DefaultOciNodeClass()
	nodePool = env.DefaultNodePool(nodeClass)
})

var _ = AfterEach(func() { env.Cleanup() })
var _ = AfterEach(func() { env.AfterEach() })

var _ = Describe("Expiration", func() {
	var dep *appsv1.Deployment
	var selector labels.Selector
	var numPods int
	BeforeEach(func() {
		numPods = 1
		dep = coretest.Deployment(coretest.DeploymentOptions{
			Replicas: int32(numPods),
			PodOptions: coretest.PodOptions{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "my-app",
					},
				},
				TerminationGracePeriodSeconds: lo.ToPtr[int64](0),
			},
		})
		selector = labels.SelectorFromSet(dep.Spec.Selector.MatchLabels)
		fmt.Println(selector.String())
	})
	It("should expire the node after the expiration is reached", func() {
		nodePool.Spec.Template.Spec.ExpireAfter = karpv1.MustParseNillableDuration("6m")
		env.ExpectCreated(nodeClass, nodePool, dep)

		nodeClaim := env.EventuallyExpectCreatedNodeClaimCount("==", 1)[0]
		node := env.EventuallyExpectCreatedNodeCount("==", 1)[0]
		env.EventuallyExpectHealthyPodCount(selector, numPods)
		env.Monitor.Reset() // Reset the monitor so that we can expect a single node to be spun up after expiration

		// Eventually the node will be tainted, which means its actively being disrupted
		Eventually(func(g Gomega) {
			g.Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(node), node)).Should(Succeed())
			_, ok := lo.Find(node.Spec.Taints, func(t corev1.Taint) bool {
				return t.MatchTaint(&karpv1.DisruptedNoScheduleTaint)
			})
			g.Expect(ok).To(BeTrue())
		}).Should(Succeed())
		// not stable
		//env.EventuallyExpectCreatedNodeCount("==", 2)
		// Set the limit to 0 to make sure we don't continue to create nodeClaims.
		// This is CRITICAL since it prevents leaking node resources into subsequent tests
		nodePool.Spec.Limits = karpv1.Limits{
			corev1.ResourceCPU: resource.MustParse("0"),
		}
		env.ExpectUpdated(nodePool)

		// After the deletion timestamp is set and all pods are drained
		// the node should be gone
		env.EventuallyExpectNotFound(nodeClaim, node)

		env.EventuallyExpectCreatedNodeClaimCount("==", 1)
		env.EventuallyExpectCreatedNodeCount("==", 1)
		env.EventuallyExpectHealthyPodCount(selector, numPods)
	})
	// todo fixme
	//It("should replace expired node with a single node and schedule all pods", func() {
	//	// Set expire after to 5 minutes since we have to respect PDB and move over pods one at a time from one node to another.
	//	// The new nodes should not expire before all the pods are moved over.
	//	nodePool.Spec.Template.Spec.ExpireAfter = karpv1.MustParseNillableDuration("5m")
	//	var numPods int32 = 2
	//	// We should setup a PDB that will only allow a minimum of 1 pod to be pending at a time
	//	minAvailable := intstr.FromInt32(numPods - 1)
	//	pdb := coretest.PodDisruptionBudget(coretest.PDBOptions{
	//		Labels: map[string]string{
	//			"app": "my-app",
	//		},
	//		MinAvailable: &minAvailable,
	//	})
	//	dep.Spec.Replicas = &numPods
	//	selector := labels.SelectorFromSet(dep.Spec.Selector.MatchLabels)
	//	env.ExpectCreated(nodeClass, nodePool, pdb, dep)
	//
	//	nodeClaim := env.EventuallyExpectCreatedNodeClaimCount("==", 1)[0]
	//	node := env.EventuallyExpectCreatedNodeCount("==", 1)[0]
	//	env.EventuallyExpectHealthyPodCount(selector, int(numPods))
	//	env.Monitor.Reset() // Reset the monitor so that we can expect a single node to be spun up after expiration
	//
	//	// Eventually the node will be tainted, which means its actively being disrupted
	//	Eventually(func(g Gomega) {
	//		g.Expect(env.Client.Get(env.Context, client.ObjectKeyFromObject(node), node)).Should(Succeed())
	//		_, ok := lo.Find(node.Spec.Taints, func(t corev1.Taint) bool {
	//			return t.MatchTaint(&karpv1.DisruptedNoScheduleTaint)
	//		})
	//		g.Expect(ok).To(BeTrue())
	//	}).Should(Succeed())
	//	// not stable
	//	env.EventuallyExpectCreatedNodeCount("==", 2)
	//	// Set the limit to 0 to make sure we don't continue to create nodeClaims.
	//	// This is CRITICAL since it prevents leaking node resources into subsequent tests
	//	nodePool.Spec.Limits = karpv1.Limits{
	//		corev1.ResourceCPU: resource.MustParse("0"),
	//	}
	//	env.ExpectUpdated(nodePool)
	//
	//	// After the deletion timestamp is set and all pods are drained
	//	// the node should be gone
	//	env.EventuallyExpectNotFound(nodeClaim, node)
	//
	//	env.EventuallyExpectCreatedNodeClaimCount("==", 1)
	//	env.EventuallyExpectCreatedNodeCount("==", 1)
	//	env.EventuallyExpectHealthyPodCount(selector, int(numPods))
	//})
})
