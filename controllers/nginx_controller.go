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

	// NGINX CRD
	var nginx webserverv1alpha1.Nginx
	if err := r.Get(ctx, req.NamespacedName, &nginx); err != nil {
		// in case of an delete request the nginx might not be found and than we can do nothing and just return a
		// successful reconciliation (all created objects are automatically garbage collected (owner references)).
		// Else the error is returned
		if client.IgnoreNotFound(err) == nil {
			logger.Info("delete request, ignoring as garbage collection deletes child resources")
			return ctrl.Result{}, nil
		}

		logger.Info("unable to load nginx")
		return ctrl.Result{}, err
	}

	// Metadata to be used in all created resources, the OwnerReferences exist to allow garbage collection
	// when the CRD is deleted
	objectMeta := metav1.ObjectMeta{
		Name:      req.Name,
		Namespace: req.Namespace,
		OwnerReferences: []metav1.OwnerReference{
			{
				APIVersion: nginx.APIVersion,
				Kind:       nginx.Kind,
				Name:       nginx.Name,
				UID:        nginx.UID,
			},
		},
	}

	// DEPLOYMENT
	deployment := &apps.Deployment{
		ObjectMeta: objectMeta,
	}

	// The CreateOrUpdate methode allows to either create a new resource or update it, preserving changes which are
	// not controlled by us. Downside is that we are not able to log the differences found.
	if _, err := ctrl.CreateOrUpdate(ctx, r.Client, deployment, func() error {
		setDeployment(deployment, req, nginx)
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

	// SERVICE
	service := &corev1.Service{
		ObjectMeta: objectMeta,
	}

	if _, err := ctrl.CreateOrUpdate(ctx, r.Client, service, func() error {

		return setService(service, nginx, deployment, req)
	}); err != nil {
		if apierrors.IsConflict(err) {
			logger.Info("Service has been changed, requeuing")
			return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
		}
		logger.Info("Service reconciliation failed")
		return ctrl.Result{}, err
	}
	logger.Info(fmt.Sprintf("successfully reconciled service %s", service.ObjectMeta.Name))

	// INGRESS
	ingress := &networking.Ingress{
		ObjectMeta: objectMeta,
	}

	if nginx.Spec.Ingress.Enabled {

		if _, err := ctrl.CreateOrUpdate(ctx, r.Client, ingress, func() error {

			return setIngress(ingress, nginx, req, service)
		}); err != nil {
			if apierrors.IsConflict(err) {
				logger.Info("Ingress has been changed, requeuing")
				return ctrl.Result{RequeueAfter: 100 * time.Millisecond}, nil
			}
			logger.Info("Ingress reconciliation failed")
			return ctrl.Result{}, err
		}
		logger.Info(fmt.Sprintf("successfully reconciled ingress %s", ingress.ObjectMeta.Name))
	} else {
		err = r.Get(ctx, req.NamespacedName, ingress)
		if err != nil {
			if client.IgnoreNotFound(err) == nil {
				logger.Info("Ingress is disabled and is not present, nothing to do")
			} else {
				return ctrl.Result{}, err
			}
		} else {
			logger.Info("Ingress is disabled and present, will be deleted")
			err = r.Delete(ctx, ingress)
			if err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	err = updateNginxStatus("Green", &nginx, r, ctx, logger)
	if err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func updateNginxStatus(status string, nginx *webserverv1alpha1.Nginx, r *NginxReconciler, ctx context.Context, logger logr.Logger) error {
	nginx.Status.Health = status
	err := r.Status().Update(ctx, nginx)
	if err != nil {
		logger.Info("Status update failed")
		return err
	}
	return nil
}

func setIngress(ingress *networking.Ingress, nginx webserverv1alpha1.Nginx, req ctrl.Request, service *corev1.Service) error {
	//ingress.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
	//	{
	//		APIVersion: nginx.APIVersion,
	//		Kind:       nginx.Kind,
	//		Name:       nginx.Name,
	//		UID:        nginx.UID,
	//	},
	//}

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
}

func setService(service *corev1.Service, nginx webserverv1alpha1.Nginx, deployment *apps.Deployment, req ctrl.Request) error {
	//service.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
	//	{
	//		APIVersion: nginx.APIVersion,
	//		Kind:       nginx.Kind,
	//		Name:       nginx.Name,
	//		UID:        nginx.UID,
	//	},
	//}

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
}

func setDeployment(deployment *apps.Deployment, req ctrl.Request, nginx webserverv1alpha1.Nginx) {
	// Selector is immutable but as we should be the only one creating this resource no need to check
	deployment.Spec.Selector = &metav1.LabelSelector{
		MatchLabels: map[string]string{
			"nginx": req.Name,
		},
	}

	//deployment.ObjectMeta.OwnerReferences = []metav1.OwnerReference{
	//	{
	//		APIVersion: nginx.APIVersion,
	//		Kind:       nginx.Kind,
	//		Name:       nginx.Name,
	//		UID:        nginx.UID,
	//	},
	//}

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
}

func (r *NginxReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&webserverv1alpha1.Nginx{}).
		Owns(&apps.Deployment{}).
		Owns(&corev1.Service{}).
		Complete(r)
}
