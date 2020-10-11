package controllers

import (
	"context"
	"k8s.io/apimachinery/pkg/types"

	//"reflect"
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	//batchv1 "k8s.io/api/batch/v1"
	//batchv1beta1 "k8s.io/api/batch/v1beta1"
	//core "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	//"k8s.io/apimachinery/pkg/types"
	apps "k8s.io/api/apps/v1"
	networking "k8s.io/api/networking/v1beta1"

	nginxv1 "github.com/mklug-org/test-operator/api/v1alpha1"
)

var _ = Describe("Nginx controller", func() {

	// Define utility constants for object names and testing timeouts/durations and intervals.
	const (
		NginxName      = "test-nginx"
		NginxNamespace = "default"

		Image   = "nginx:v1.0.1"
		Message = "Hello test message"

		IngressEnabled  = true
		IngressHostname = "test.example.com"

		timeout  = time.Second * 10
		interval = time.Millisecond * 250
	)

	Context("When creating Nginx", func() {
		It("deployment and ingress should be created correctly", func() {

			replicas := int32(3)
			ctx := context.Background()
			nginx := &nginxv1.Nginx{
				TypeMeta: metav1.TypeMeta{
					APIVersion: "webserver.mklug.at/v1alpha1",
					Kind:       "Nginx",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:      NginxName,
					Namespace: NginxNamespace,
				},
				Spec: nginxv1.NginxSpec{
					Replicas: &replicas,
					Image:    Image,
					Message:  Message,
					Ingress: nginxv1.IngressSpec{
						Enabled:  IngressEnabled,
						Hostname: IngressHostname,
					},
				},
			}
			Expect(k8sClient.Create(ctx, nginx)).Should(Succeed())

			nginxLookupKey := types.NamespacedName{Name: NginxName, Namespace: NginxNamespace}
			createdNginx := &nginxv1.Nginx{}

			// We'll need to retry getting this newly created Nginx, given that creation may not immediately happen.
			Eventually(func() bool {
				err := k8sClient.Get(ctx, nginxLookupKey, createdNginx)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())
			// Let's make sure our Nginx field values were properly converted/handled.
			Expect(createdNginx.Spec.Replicas).Should(Equal(&replicas))
			Expect(createdNginx.Spec.Image).Should(Equal(Image))
			Expect(createdNginx.Spec.Message).Should(Equal(Message))
			Expect(createdNginx.Spec.Ingress.Enabled).Should(Equal(IngressEnabled))
			Expect(createdNginx.Spec.Ingress.Hostname).Should(Equal(IngressHostname))

			// We'll try to get the deployment that the Nginx controller should have created
			deploymentLookupKey := types.NamespacedName{Name: NginxName, Namespace: NginxNamespace}
			deploymentNginx := &apps.Deployment{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, deploymentLookupKey, deploymentNginx)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			Expect(deploymentNginx.Spec.Replicas).Should(Equal(&replicas))
			Expect(deploymentNginx.Spec.Template.Spec.Containers[0].Ports[0].ContainerPort).Should(Equal(int32(80)))

			// We'll try to get the ingress that the Nginx controller should have created
			ingressLookupKey := types.NamespacedName{Name: NginxName, Namespace: NginxNamespace}
			ingressNginx := &networking.Ingress{}

			Eventually(func() bool {
				err := k8sClient.Get(ctx, ingressLookupKey, ingressNginx)
				if err != nil {
					return false
				}
				return true
			}, timeout, interval).Should(BeTrue())

			Expect(ingressNginx.Spec.Rules[0].Host).Should(Equal(IngressHostname))
		})
	})
})
