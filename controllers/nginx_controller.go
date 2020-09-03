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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

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

		//err = ctrl.SetControllerReference(&nginx, deployment, r.Scheme)
		//if err != nil {
		//	logger.Error(err, "Setting Controller reference failed")
		//	return err
		//}

		// Selector is immutable but as we should be the only one creating this resource no need to check
		deployment.Spec.Selector = &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"nginx": req.Name,
			},
		}

		deployment.Spec.Replicas = nginx.Spec.Replicas
		deployment.Spec.Template = corev1.PodTemplateSpec{
			ObjectMeta: metav1.ObjectMeta{
				Labels: map[string]string{
					"nginx": req.Name,
				},
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion: nginx.APIVersion,
						Kind:       nginx.Kind,
						Name:       nginx.Name,
						UID:        nginx.UID,
					},
				},
			},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{
					{
						Name:  "nginx",
						Image: nginx.Spec.Image,
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
		logger.Info(fmt.Sprintf("successfully reconciled deployment %s", req.Name))

		return nil
	}); err != nil {
		logger.Error(err, "Deployment reconciliation failed")
		return ctrl.Result{}, err
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: req.Namespace,
		},
	}

	if _, err := ctrl.CreateOrUpdate(ctx, r.Client, service, func() error {

		//err = ctrl.SetControllerReference(&nginx, service, r.Scheme)
		//if err != nil {
		//	logger.Error(err, "Setting Controller reference failed")
		//	return err
		//}

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
				Name: "http",
				Port: 80,
				TargetPort: intstr.IntOrString{
					StrVal: "http",
				},
			},
		}
		service.Spec.Selector = map[string]string{
			"nginx": req.Name,
		}
		logger.Info(fmt.Sprintf("successfully reconciled service %s", req.Name))

		return nil
	}); err != nil {
		logger.Error(err, "Service reconciliation failed")
		return ctrl.Result{}, err
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
