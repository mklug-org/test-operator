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

package controllers

import (
	"context"
	"fmt"
	"github.com/go-logr/logr"
	apps "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networking "k8s.io/api/networking/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"time"

	webserverv1alpha1 "github.com/mklug-org/test-operator/api/v1alpha1"
)

// NginxReconciler reconciles a Nginx object
type NginxReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=webserver.mklug.at,resources=nginxes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=webserver.mklug.at,resources=nginxes/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,resources=deployments/status,verbs=get
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="networking.k8s.io",resources=ingresses,verbs=get;list;watch;create;update;patch;delete

func (r *NginxReconciler) Reconcile(req ctrl.Request) (ctrl.Result, error) {
	var err error
	ctx := context.Background()
	logger := r.Log.WithValues("nginx", req.NamespacedName)

	logger.Info("Reconciling")

	var nginx webserverv1alpha1.Nginx
	if err := r.Get(ctx, req.NamespacedName, &nginx); err != nil {
		// in case of an delete request the nginx might not be found and than we can do nothing and just return a
		// successful reconciliation (all created objects are automatically garbage collected (owner references)).
		// Else the error is returned
		if client.IgnoreNotFound(err) == nil {
			logger.Info("delete request, ignoring as garbage collection deletes objects")
			return ctrl.Result{}, nil
		}

		logger.Error(err, "unable to load nginx")
		return ctrl.Result{}, err
	}

	deployment := &apps.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
	}

	// The CreateOrUpdate methode allows to either create a new resource or update it, preserving changes which are
	// not controlled by us. Downside is that we are not able to log the differences found.
	if _, err := ctrl.CreateOrUpdate(ctx, r.Client, deployment, func() error {

		// Selector is immutable but as we should be the only one creating this resource no need to check
		deployment.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"nginx": req.Name,
			},
		}

		deployment.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: nginx.APIVersion,
				Kind:       nginx.Kind,
				Name:       nginx.Name,
				UID:        nginx.UID,
			},
		}
		deployment.Spec.Replicas = nginx.Spec.Replicas
		deployment.Spec.Template = corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"nginx": req.Name,
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: nginx.Spec.Image,
						Args: []string{
							"sh",
							"-c",
							fmt.Sprintf("echo '%s' > /usr/share/nginx/html/index.html && nginx -g 'daemon off;'", nginx.Spec.Message),
						},
						Ports: []corev1.ContainerPort{
							{
								Name:          "http",
								ContainerPort: 80,
								Protocol:      "TCP",
							},
						},
					},
				},
			},
		}
		return nil
	}); err != nil {
		// After updating the resource the Reconcile function kicks in and it could get an older cached version,
		// should this be the case just requeue
		if apierrors.IsConflict(err) {
			logger.Info("Deployment has been changed, requeuing")
			return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
		}
		logger.Info("Deployment reconciliation failed")
		return ctrl.Result{}, err
	}
	logger.Info(fmt.Sprintf("successfully reconciled deployment %s", deployment.ObjectMeta.Name))

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
	}

	if _, err := ctrl.CreateOrUpdate(ctx, r.Client, service, func() error {

		service.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
			{
				APIVersion: nginx.APIVersion,
				Kind:       nginx.Kind,
				Name:       nginx.Name,
				UID:        nginx.UID,
			},
		}

		service.Spec.Ports = []corev1.ServicePort{
			{
				Name: deployment.Spec.Template.Spec.Containers[0].Name,
				Port: 80,
				TargetPort: intstr.IntOrString{
					StrVal: deployment.Spec.Template.Spec.Containers[0].Name,
				},
			},
		}
		service.Spec.Selector = map[string]string{
			"nginx": req.Name,
		}

		return nil
	}); err != nil {
		if apierrors.IsConflict(err) {
			// After updating the resource the Reconcile function kicks in and it could get an older cached version,
			// should this be the case just requeue
			logger.Info("Service has been changed, requeuing")
			return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
		}
		logger.Error(err, "Service reconciliation failed")
		return ctrl.Result{}, err
	}
	logger.Info(fmt.Sprintf("successfully reconciled service %s", service.ObjectMeta.Name))

	// disabling afterwards will nor work
	if nginx.Spec.Ingress.Enabled {

		ingress := &networking.Ingress{
			ObjectMeta: metav1.ObjectMeta{
				Name:      req.Name,
				Namespace: req.Namespace,
			},
		}

		if _, err := ctrl.CreateOrUpdate(ctx, r.Client, ingress, func() error {

			ingress.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
				{
					APIVersion: nginx.APIVersion,
					Kind:       nginx.Kind,
					Name:       nginx.Name,
					UID:        nginx.UID,
				},
			}

			ingress.Spec.Rules = []networking.IngressRule{
				{
					Host: nginx.Spec.Ingress.Hostname,
					IngressRuleValue: networking.IngressRuleValue{
						HTTP: &networking.HTTPIngressRuleValue{
							Paths: []networking.HTTPIngressPath{
								{
									Backend: networking.IngressBackend{
										ServiceName: req.Name,
										ServicePort: service.Spec.Ports[0].TargetPort,
									},
								},
							},
						},
					},
				},
			}
			return nil
		}); err != nil {
			if apierrors.IsConflict(err) {
				// After updating the resource the Reconcile function kicks in and it could get an older cached version,
				// should this be the case just requeue
				logger.Info("Service has been changed, requeuing")
				return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
			}
			logger.Error(err, "Ingress reconciliation failed")
			return ctrl.Result{}, err
		}
		logger.Info(fmt.Sprintf("successfully reconciled ingress %s", ingress.ObjectMeta.Name))
	}

	nginx.Status.Health = "Green"
	err = r.Status().Update(ctx, &nginx)
	if err != nil {
		logger.Error(err, "Status update failed")
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *NginxReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&webserverv1alpha1.Nginx{}).
		Owns(&apps.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
