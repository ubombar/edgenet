package subnamespace

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"testing"
	"time"

	corev1alpha "github.com/EdgeNet-project/edgenet/pkg/apis/core/v1alpha"
	"github.com/EdgeNet-project/edgenet/pkg/generated/clientset/versioned"
	edgenettestclient "github.com/EdgeNet-project/edgenet/pkg/generated/clientset/versioned/fake"
	informers "github.com/EdgeNet-project/edgenet/pkg/generated/informers/externalversions"
	"github.com/EdgeNet-project/edgenet/pkg/signals"
	"github.com/EdgeNet-project/edgenet/pkg/util"
	"github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	testclient "k8s.io/client-go/kubernetes/fake"
	"k8s.io/klog"
)

// The main structure of test group
type TestGroup struct {
	tenantObj        corev1alpha.Tenant
	trqObj           corev1alpha.TenantResourceQuota
	resourceQuotaObj corev1.ResourceQuota
	subNamespaceObj  corev1alpha.SubNamespace
}

var controller *Controller
var kubeclientset kubernetes.Interface = testclient.NewSimpleClientset()
var edgenetclientset versioned.Interface = edgenettestclient.NewSimpleClientset()

func TestMain(m *testing.M) {
	klog.SetOutput(ioutil.Discard)
	log.SetOutput(ioutil.Discard)
	logrus.SetOutput(ioutil.Discard)

	flag.String("dir", "../../../../..", "Override the directory.")
	flag.String("smtp-path", "../../../../../configs/smtp_test.yaml", "Set SMTP path.")
	flag.Parse()

	stopCh := signals.SetupSignalHandler()

	go func() {
		edgenetInformerFactory := informers.NewSharedInformerFactory(edgenetclientset, time.Second*30)

		newController := NewController(kubeclientset,
			edgenetclientset,
			edgenetInformerFactory.Core().V1alpha().SubNamespaces())

		edgenetInformerFactory.Start(stopCh)
		controller = newController
		if err := controller.Run(2, stopCh); err != nil {
			klog.Fatalf("Error running controller: %s", err.Error())
		}
	}()

	os.Exit(m.Run())
	<-stopCh
}

// Init syncs the test group
func (g *TestGroup) Init() {
	tenantObj := corev1alpha.Tenant{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Tenant",
			APIVersion: "core.edgenet.io/v1alpha",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "edgenet",
		},
		Spec: corev1alpha.TenantSpec{
			FullName:  "EdgeNet",
			ShortName: "EdgeNet",
			URL:       "https://www.edge-net.org",
			Address: corev1alpha.Address{
				City:    "Paris - NY - CA",
				Country: "France - US",
				Street:  "4 place Jussieu, boite 169",
				ZIP:     "75005",
			},
			Contact: corev1alpha.Contact{
				Email:     "john.doe@edge-net.org",
				FirstName: "John",
				LastName:  "Doe",
				Phone:     "+33NUMBER",
				Handle:    "johndoe",
			},
			Enabled: true,
		},
	}
	trqObj := corev1alpha.TenantResourceQuota{
		TypeMeta: metav1.TypeMeta{
			Kind:       "TenantResourceQuota",
			APIVersion: "core.edgenet.io/v1alpha",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "edgenet",
		},
		Spec: corev1alpha.TenantResourceQuotaSpec{
			Claim: map[string]corev1alpha.ResourceTuning{
				"initial": {
					ResourceList: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("8000m"),
						corev1.ResourceMemory: resource.MustParse("8192Mi"),
					},
				},
			},
		},
	}
	resourceQuotaObj := corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name: "core-quota",
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: map[corev1.ResourceName]resource.Quantity{
				"cpu":              resource.MustParse("8000m"),
				"memory":           resource.MustParse("8192Mi"),
				"requests.storage": resource.MustParse("8Gi"),
			},
		},
	}
	subNamespaceObj := corev1alpha.SubNamespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "SubNamespace",
			APIVersion: "core.edgenet.io/v1alpha",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "edgenet-sub",
			Namespace: "edgenet",
		},
		Spec: corev1alpha.SubNamespaceSpec{
			Resources: corev1alpha.Resources{
				CPU:    "6000m",
				Memory: "6Gi",
			},
			Inheritance: corev1alpha.Inheritance{
				RBAC:          true,
				NetworkPolicy: true,
			},
		},
	}

	g.tenantObj = tenantObj
	g.trqObj = trqObj
	g.resourceQuotaObj = resourceQuotaObj
	g.subNamespaceObj = subNamespaceObj

	// Imitate tenant creation processes
	edgenetclientset.CoreV1alpha().Tenants().Create(context.TODO(), g.tenantObj.DeepCopy(), metav1.CreateOptions{})
	namespace := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: g.tenantObj.GetName()}}
	namespaceLabels := map[string]string{"edge-net.io/generated": "true", "edge-net.io/tenant": g.tenantObj.GetName(), "edge-net.io/kind": "core"}
	namespace.SetLabels(namespaceLabels)
	kubeclientset.CoreV1().Namespaces().Create(context.TODO(), &namespace, metav1.CreateOptions{})
	edgenetclientset.CoreV1alpha().TenantResourceQuotas().Create(context.TODO(), g.trqObj.DeepCopy(), metav1.CreateOptions{})
	kubeclientset.CoreV1().ResourceQuotas(namespace.GetName()).Create(context.TODO(), g.resourceQuotaObj.DeepCopy(), metav1.CreateOptions{})
	kubeclientset.RbacV1().Roles(namespace.GetName()).Create(context.TODO(), &rbacv1.Role{ObjectMeta: metav1.ObjectMeta{Name: "edgenet-test"}}, metav1.CreateOptions{})
	kubeclientset.RbacV1().RoleBindings(namespace.GetName()).Create(context.TODO(), &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "edgenet-test"}}, metav1.CreateOptions{})
	kubeclientset.NetworkingV1().NetworkPolicies(namespace.GetName()).Create(context.TODO(), &networkingv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: "edgenet-test"}}, metav1.CreateOptions{})
}

func TestStartController(t *testing.T) {
	g := TestGroup{}
	g.Init()

	coreResourceQuota, err := kubeclientset.CoreV1().ResourceQuotas(g.tenantObj.GetName()).Get(context.TODO(), fmt.Sprintf("core-quota"), metav1.GetOptions{})
	util.OK(t, err)
	coreQuotaCPU := coreResourceQuota.Spec.Hard.Cpu().Value()
	coreQuotaMemory := coreResourceQuota.Spec.Hard.Memory().Value()

	// Create a subnamespace
	subNamespaceControllerTest := g.subNamespaceObj.DeepCopy()
	subNamespaceControllerTest.SetName("subnamespace-controller")
	_, err = edgenetclientset.CoreV1alpha().SubNamespaces(g.tenantObj.GetName()).Create(context.TODO(), subNamespaceControllerTest, metav1.CreateOptions{})
	util.OK(t, err)
	// Wait for the status update of the created object
	time.Sleep(time.Millisecond * 500)
	// Get the object and check the status
	_, err = kubeclientset.CoreV1().Namespaces().Get(context.TODO(), fmt.Sprintf("%s-%s", g.tenantObj.GetName(), subNamespaceControllerTest.GetName()), metav1.GetOptions{})
	util.OK(t, err)
	tunedCoreResourceQuota, err := kubeclientset.CoreV1().ResourceQuotas(g.tenantObj.GetName()).Get(context.TODO(), fmt.Sprintf("core-quota"), metav1.GetOptions{})
	util.OK(t, err)
	tunedCoreQuotaCPU := tunedCoreResourceQuota.Spec.Hard.Cpu().Value()
	tunedCoreQuotaMemory := tunedCoreResourceQuota.Spec.Hard.Memory().Value()

	cpuResource := resource.MustParse(subNamespaceControllerTest.Spec.Resources.CPU)
	cpuDemand := cpuResource.Value()
	memoryResource := resource.MustParse(subNamespaceControllerTest.Spec.Resources.Memory)
	memoryDemand := memoryResource.Value()

	util.Equals(t, coreQuotaCPU-cpuDemand, tunedCoreQuotaCPU)
	util.Equals(t, coreQuotaMemory-memoryDemand, tunedCoreQuotaMemory)

	subResourceQuota, err := kubeclientset.CoreV1().ResourceQuotas(fmt.Sprintf("%s-%s", g.tenantObj.GetName(), subNamespaceControllerTest.GetName())).Get(context.TODO(), fmt.Sprintf("sub-quota"), metav1.GetOptions{})
	util.OK(t, err)
	subQuotaCPU := subResourceQuota.Spec.Hard.Cpu().Value()
	subQuotaMemory := subResourceQuota.Spec.Hard.Memory().Value()
	util.Equals(t, int64(6), subQuotaCPU)
	util.Equals(t, int64(6442450944), subQuotaMemory)

	subNamespaceControllerNestedTest := g.subNamespaceObj.DeepCopy()
	subNamespaceControllerNestedTest.Spec.Resources.CPU = "1000m"
	subNamespaceControllerNestedTest.Spec.Resources.Memory = "1Gi"
	subNamespaceControllerNestedTest.SetName("subnamespace-controller-nested")
	subNamespaceControllerNestedTest.SetNamespace(fmt.Sprintf("%s-%s", g.tenantObj.GetName(), subNamespaceControllerTest.GetName()))
	_, err = edgenetclientset.CoreV1alpha().SubNamespaces(subNamespaceControllerNestedTest.GetNamespace()).Create(context.TODO(), subNamespaceControllerNestedTest, metav1.CreateOptions{})
	util.OK(t, err)
	// Wait for the status update of the created object
	time.Sleep(time.Millisecond * 500)

	subResourceQuota, err = kubeclientset.CoreV1().ResourceQuotas(fmt.Sprintf("%s-%s", g.tenantObj.GetName(), subNamespaceControllerTest.GetName())).Get(context.TODO(), fmt.Sprintf("sub-quota"), metav1.GetOptions{})
	util.OK(t, err)
	subQuotaCPU = subResourceQuota.Spec.Hard.Cpu().Value()
	subQuotaMemory = subResourceQuota.Spec.Hard.Memory().Value()
	util.Equals(t, int64(5), subQuotaCPU)
	util.Equals(t, int64(5368709120), subQuotaMemory)

	tunedCoreResourceQuota, err = kubeclientset.CoreV1().ResourceQuotas(g.tenantObj.GetName()).Get(context.TODO(), fmt.Sprintf("core-quota"), metav1.GetOptions{})
	util.OK(t, err)
	tunedCoreQuotaCPU = tunedCoreResourceQuota.Spec.Hard.Cpu().Value()
	tunedCoreQuotaMemory = tunedCoreResourceQuota.Spec.Hard.Memory().Value()
	util.Equals(t, int64(2), tunedCoreQuotaCPU)
	util.Equals(t, int64(2147483648), tunedCoreQuotaMemory)

	nestedSubResourceQuota, err := kubeclientset.CoreV1().ResourceQuotas(fmt.Sprintf("%s-%s", g.tenantObj.GetName(), subNamespaceControllerNestedTest.GetName())).Get(context.TODO(), fmt.Sprintf("sub-quota"), metav1.GetOptions{})
	util.OK(t, err)
	nestedSubQuotaCPU := nestedSubResourceQuota.Spec.Hard.Cpu().Value()
	nestedSubQuotaMemory := nestedSubResourceQuota.Spec.Hard.Memory().Value()
	util.Equals(t, int64(1), nestedSubQuotaCPU)
	util.Equals(t, int64(1073741824), nestedSubQuotaMemory)

	err = edgenetclientset.CoreV1alpha().SubNamespaces(g.tenantObj.GetName()).Delete(context.TODO(), subNamespaceControllerTest.GetName(), metav1.DeleteOptions{})
	util.OK(t, err)
	time.Sleep(time.Millisecond * 500)
	_, err = kubeclientset.CoreV1().Namespaces().Get(context.TODO(), fmt.Sprintf("%s-%s", g.tenantObj.GetName(), subNamespaceControllerTest.GetName()), metav1.GetOptions{})
	util.Equals(t, true, errors.IsNotFound(err))
	latestParentResourceQuota, err := kubeclientset.CoreV1().ResourceQuotas(g.tenantObj.GetName()).Get(context.TODO(), fmt.Sprintf("core-quota"), metav1.GetOptions{})
	util.OK(t, err)
	latestParentQuotaCPU := latestParentResourceQuota.Spec.Hard.Cpu().Value()
	latestParentQuotaMemory := latestParentResourceQuota.Spec.Hard.Memory().Value()
	util.Equals(t, coreQuotaCPU, latestParentQuotaCPU)
	util.Equals(t, coreQuotaMemory, latestParentQuotaMemory)
}

func TestCreate(t *testing.T) {
	g := TestGroup{}
	g.Init()

	subnamespace1 := g.subNamespaceObj.DeepCopy()
	subnamespace1.SetName("all")
	subnamespace1.Spec.Resources.CPU = "2000m"
	subnamespace1.Spec.Resources.Memory = "2Gi"
	subnamespace1nested := g.subNamespaceObj.DeepCopy()
	subnamespace1nested.SetName("all-nested")
	subnamespace1nested.Spec.Resources.CPU = "1000m"
	subnamespace1nested.Spec.Resources.Memory = "1Gi"
	subnamespace1nested.SetNamespace(fmt.Sprintf("%s-%s", g.tenantObj.GetName(), subnamespace1.GetName()))
	subnamespace2 := g.subNamespaceObj.DeepCopy()
	subnamespace2.SetName("rbac")
	subnamespace2.Spec.Inheritance.NetworkPolicy = false
	subnamespace2.Spec.Resources.CPU = "1000m"
	subnamespace2.Spec.Resources.Memory = "1Gi"
	subnamespace3 := g.subNamespaceObj.DeepCopy()
	subnamespace3.SetName("networkpolicy")
	subnamespace3.Spec.Inheritance.RBAC = false
	subnamespace3.Spec.Resources.CPU = "1000m"
	subnamespace3.Spec.Resources.Memory = "1Gi"
	subnamespace4 := g.subNamespaceObj.DeepCopy()
	subnamespace4.SetName("expiry")
	subnamespace4.Spec.Resources.CPU = "1000m"
	subnamespace4.Spec.Resources.Memory = "1Gi"

	t.Run("inherit all without expiry date", func(t *testing.T) {
		defer edgenetclientset.CoreV1alpha().SubNamespaces(g.tenantObj.GetName()).Delete(context.TODO(), subnamespace1.GetName(), metav1.DeleteOptions{})

		_, err := edgenetclientset.CoreV1alpha().SubNamespaces(g.tenantObj.GetName()).Create(context.TODO(), subnamespace1, metav1.CreateOptions{})
		util.OK(t, err)
		time.Sleep(500 * time.Millisecond)
		childNamespace, err := kubeclientset.CoreV1().Namespaces().Get(context.TODO(), fmt.Sprintf("%s-%s", g.tenantObj.GetName(), subnamespace1.GetName()), metav1.GetOptions{})
		util.OK(t, err)

		t.Run("check core resource quota", func(t *testing.T) {
			coreResourceQuota, _ := kubeclientset.CoreV1().ResourceQuotas(g.tenantObj.GetName()).Get(context.TODO(), "core-quota", metav1.GetOptions{})
			util.Equals(t, int64(6), coreResourceQuota.Spec.Hard.Cpu().Value())
			util.Equals(t, int64(6442450944), coreResourceQuota.Spec.Hard.Memory().Value())
		})

		t.Run("check sub resource quota", func(t *testing.T) {
			subResourceQuota, _ := kubeclientset.CoreV1().ResourceQuotas(childNamespace.GetName()).Get(context.TODO(), "sub-quota", metav1.GetOptions{})
			util.Equals(t, int64(2), subResourceQuota.Spec.Hard.Cpu().Value())
			util.Equals(t, int64(2147483648), subResourceQuota.Spec.Hard.Memory().Value())
			t.Run("nested subnamespaces", func(t *testing.T) {
				_, err := edgenetclientset.CoreV1alpha().SubNamespaces(childNamespace.GetName()).Create(context.TODO(), subnamespace1nested, metav1.CreateOptions{})
				util.OK(t, err)
				time.Sleep(500 * time.Millisecond)
				nestedChildNamespace, err := kubeclientset.CoreV1().Namespaces().Get(context.TODO(), fmt.Sprintf("%s-%s", g.tenantObj.GetName(), subnamespace1nested.GetName()), metav1.GetOptions{})
				util.OK(t, err)

				subResourceQuota, _ := kubeclientset.CoreV1().ResourceQuotas(childNamespace.GetName()).Get(context.TODO(), "sub-quota", metav1.GetOptions{})
				util.Equals(t, int64(1), subResourceQuota.Spec.Hard.Cpu().Value())
				util.Equals(t, int64(1073741824), subResourceQuota.Spec.Hard.Memory().Value())

				nestedSubResourceQuota, _ := kubeclientset.CoreV1().ResourceQuotas(nestedChildNamespace.GetName()).Get(context.TODO(), "sub-quota", metav1.GetOptions{})
				util.Equals(t, int64(1), nestedSubResourceQuota.Spec.Hard.Cpu().Value())
				util.Equals(t, int64(1073741824), nestedSubResourceQuota.Spec.Hard.Memory().Value())
			})
		})

		if roleRaw, err := kubeclientset.RbacV1().Roles(subnamespace1.GetNamespace()).List(context.TODO(), metav1.ListOptions{}); err == nil {
			// TODO: Provide err information at the status
			for _, roleRow := range roleRaw.Items {
				_, err := kubeclientset.RbacV1().Roles(childNamespace.GetName()).Get(context.TODO(), roleRow.GetName(), metav1.GetOptions{})
				util.OK(t, err)
			}
		}
		if roleBindingRaw, err := kubeclientset.RbacV1().RoleBindings(subnamespace1.GetNamespace()).List(context.TODO(), metav1.ListOptions{}); err == nil {
			// TODO: Provide err information at the status
			for _, roleBindingRow := range roleBindingRaw.Items {
				_, err := kubeclientset.RbacV1().RoleBindings(childNamespace.GetName()).Get(context.TODO(), roleBindingRow.GetName(), metav1.GetOptions{})
				util.OK(t, err)
			}
		}
		if networkPolicyRaw, err := kubeclientset.NetworkingV1().NetworkPolicies(subnamespace1.GetNamespace()).List(context.TODO(), metav1.ListOptions{}); err == nil {
			// TODO: Provide err information at the status
			for _, networkPolicyRow := range networkPolicyRaw.Items {
				_, err := kubeclientset.NetworkingV1().NetworkPolicies(childNamespace.GetName()).Get(context.TODO(), networkPolicyRow.GetName(), metav1.GetOptions{})
				util.OK(t, err)
			}
		}
	})
	t.Run("inherit rbac without expiry date", func(t *testing.T) {
		defer edgenetclientset.CoreV1alpha().SubNamespaces(g.tenantObj.GetName()).Delete(context.TODO(), subnamespace2.GetName(), metav1.DeleteOptions{})

		_, err := edgenetclientset.CoreV1alpha().SubNamespaces(g.tenantObj.GetName()).Create(context.TODO(), subnamespace2, metav1.CreateOptions{})
		util.OK(t, err)
		time.Sleep(500 * time.Millisecond)
		childNamespace, err := kubeclientset.CoreV1().Namespaces().Get(context.TODO(), fmt.Sprintf("%s-%s", g.tenantObj.GetName(), subnamespace2.GetName()), metav1.GetOptions{})
		util.OK(t, err)
		if roleRaw, err := kubeclientset.RbacV1().Roles(subnamespace2.GetNamespace()).List(context.TODO(), metav1.ListOptions{}); err == nil && subnamespace2.Spec.Inheritance.RBAC {
			// TODO: Provide err information at the status
			for _, roleRow := range roleRaw.Items {
				_, err := kubeclientset.RbacV1().Roles(childNamespace.GetName()).Get(context.TODO(), roleRow.GetName(), metav1.GetOptions{})
				util.OK(t, err)
			}
		}
		if roleBindingRaw, err := kubeclientset.RbacV1().RoleBindings(subnamespace2.GetNamespace()).List(context.TODO(), metav1.ListOptions{}); err == nil && subnamespace2.Spec.Inheritance.RBAC {
			// TODO: Provide err information at the status
			for _, roleBindingRow := range roleBindingRaw.Items {
				_, err := kubeclientset.RbacV1().RoleBindings(childNamespace.GetName()).Get(context.TODO(), roleBindingRow.GetName(), metav1.GetOptions{})
				util.OK(t, err)
			}
		}
		if networkPolicyRaw, err := kubeclientset.NetworkingV1().NetworkPolicies(subnamespace2.GetNamespace()).List(context.TODO(), metav1.ListOptions{}); err == nil && subnamespace2.Spec.Inheritance.NetworkPolicy {
			// TODO: Provide err information at the status
			for _, networkPolicyRow := range networkPolicyRaw.Items {
				_, err := kubeclientset.NetworkingV1().NetworkPolicies(childNamespace.GetName()).Get(context.TODO(), networkPolicyRow.GetName(), metav1.GetOptions{})
				util.Equals(t, true, errors.IsNotFound(err))
			}
		}
	})
	t.Run("inherit networkpolicy without expiry date", func(t *testing.T) {
		defer edgenetclientset.CoreV1alpha().SubNamespaces(g.tenantObj.GetName()).Delete(context.TODO(), subnamespace3.GetName(), metav1.DeleteOptions{})

		_, err := edgenetclientset.CoreV1alpha().SubNamespaces(g.tenantObj.GetName()).Create(context.TODO(), subnamespace3, metav1.CreateOptions{})
		util.OK(t, err)
		time.Sleep(500 * time.Millisecond)
		childNamespace, err := kubeclientset.CoreV1().Namespaces().Get(context.TODO(), fmt.Sprintf("%s-%s", g.tenantObj.GetName(), subnamespace3.GetName()), metav1.GetOptions{})
		util.OK(t, err)
		if roleRaw, err := kubeclientset.RbacV1().Roles(subnamespace3.GetNamespace()).List(context.TODO(), metav1.ListOptions{}); err == nil && subnamespace3.Spec.Inheritance.RBAC {
			// TODO: Provide err information at the status
			for _, roleRow := range roleRaw.Items {
				_, err := kubeclientset.RbacV1().Roles(childNamespace.GetName()).Get(context.TODO(), roleRow.GetName(), metav1.GetOptions{})
				util.Equals(t, true, errors.IsNotFound(err))

			}
		}
		if roleBindingRaw, err := kubeclientset.RbacV1().RoleBindings(subnamespace3.GetNamespace()).List(context.TODO(), metav1.ListOptions{}); err == nil && subnamespace3.Spec.Inheritance.RBAC {
			// TODO: Provide err information at the status
			for _, roleBindingRow := range roleBindingRaw.Items {
				_, err := kubeclientset.RbacV1().RoleBindings(childNamespace.GetName()).Get(context.TODO(), roleBindingRow.GetName(), metav1.GetOptions{})
				util.Equals(t, true, errors.IsNotFound(err))
			}
		}
		if networkPolicyRaw, err := kubeclientset.NetworkingV1().NetworkPolicies(subnamespace3.GetNamespace()).List(context.TODO(), metav1.ListOptions{}); err == nil && subnamespace3.Spec.Inheritance.NetworkPolicy {
			// TODO: Provide err information at the status
			for _, networkPolicyRow := range networkPolicyRaw.Items {
				_, err := kubeclientset.NetworkingV1().NetworkPolicies(childNamespace.GetName()).Get(context.TODO(), networkPolicyRow.GetName(), metav1.GetOptions{})
				util.OK(t, err)
			}
		}
	})
	t.Run("inherit all with expiry date", func(t *testing.T) {
		subnamespace4.Spec.Expiry = &metav1.Time{
			Time: time.Now().Add(700 * time.Millisecond),
		}
		_, err := edgenetclientset.CoreV1alpha().SubNamespaces(g.tenantObj.GetName()).Create(context.TODO(), subnamespace4, metav1.CreateOptions{})
		util.OK(t, err)
		time.Sleep(500 * time.Millisecond)
		_, err = kubeclientset.CoreV1().Namespaces().Get(context.TODO(), fmt.Sprintf("%s-%s", g.tenantObj.GetName(), subnamespace4.GetName()), metav1.GetOptions{})
		util.OK(t, err)
		time.Sleep(500 * time.Millisecond)
		_, err = kubeclientset.CoreV1().Namespaces().Get(context.TODO(), fmt.Sprintf("%s-%s", g.tenantObj.GetName(), subnamespace4.GetName()), metav1.GetOptions{})
		util.Equals(t, true, errors.IsNotFound(err))
	})
}

func TestQuota(t *testing.T) {
	g := TestGroup{}
	g.Init()

	subnamespace1 := g.subNamespaceObj.DeepCopy()
	subnamespace1.SetName("all")
	subnamespace2 := g.subNamespaceObj.DeepCopy()
	subnamespace2.SetName("rbac")
	subnamespace2.Spec.Inheritance.NetworkPolicy = false
	subnamespace3 := g.subNamespaceObj.DeepCopy()
	subnamespace3.SetName("networkpolicy")
	subnamespace3.Spec.Inheritance.RBAC = false

	_, err := edgenetclientset.CoreV1alpha().SubNamespaces(g.tenantObj.GetName()).Create(context.TODO(), subnamespace1, metav1.CreateOptions{})
	util.OK(t, err)
	time.Sleep(500 * time.Millisecond)
	_, err = kubeclientset.CoreV1().Namespaces().Get(context.TODO(), fmt.Sprintf("%s-%s", g.tenantObj.GetName(), subnamespace1.GetName()), metav1.GetOptions{})
	util.OK(t, err)

	_, err = edgenetclientset.CoreV1alpha().SubNamespaces(g.tenantObj.GetName()).Create(context.TODO(), subnamespace2, metav1.CreateOptions{})
	util.OK(t, err)
	time.Sleep(500 * time.Millisecond)
	_, err = kubeclientset.CoreV1().Namespaces().Get(context.TODO(), fmt.Sprintf("%s-%s", g.tenantObj.GetName(), subnamespace2.GetName()), metav1.GetOptions{})
	util.Equals(t, true, errors.IsNotFound(err))

	_, err = edgenetclientset.CoreV1alpha().SubNamespaces(g.tenantObj.GetName()).Create(context.TODO(), subnamespace3, metav1.CreateOptions{})
	util.OK(t, err)
	time.Sleep(500 * time.Millisecond)
	_, err = kubeclientset.CoreV1().Namespaces().Get(context.TODO(), fmt.Sprintf("%s-%s", g.tenantObj.GetName(), subnamespace3.GetName()), metav1.GetOptions{})
	util.Equals(t, true, errors.IsNotFound(err))
}
