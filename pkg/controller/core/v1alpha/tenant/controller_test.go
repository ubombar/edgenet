package tenant

import (
	"context"
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"testing"
	"time"

	"github.com/EdgeNet-project/edgenet/pkg/access"
	corev1alpha "github.com/EdgeNet-project/edgenet/pkg/apis/core/v1alpha"
	"github.com/EdgeNet-project/edgenet/pkg/generated/clientset/versioned"
	edgenettestclient "github.com/EdgeNet-project/edgenet/pkg/generated/clientset/versioned/fake"
	informers "github.com/EdgeNet-project/edgenet/pkg/generated/informers/externalversions"
	"github.com/EdgeNet-project/edgenet/pkg/signals"
	"github.com/EdgeNet-project/edgenet/pkg/util"
	"github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	kubeinformers "k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	testclient "k8s.io/client-go/kubernetes/fake"
	"k8s.io/klog"
)

type TestGroup struct {
	tenantObj corev1alpha.Tenant
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
		kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeclientset, time.Second*30)
		edgenetInformerFactory := informers.NewSharedInformerFactory(edgenetclientset, time.Second*30)

		newController := NewController(kubeclientset,
			edgenetclientset,
			edgenetInformerFactory.Core().V1alpha().Tenants())

		kubeInformerFactory.Start(stopCh)
		edgenetInformerFactory.Start(stopCh)
		controller = newController
		if err := controller.Run(2, stopCh); err != nil {
			klog.Fatalf("Error running controller: %s", err.Error())
		}
	}()

	access.Clientset = kubeclientset
	kubeSystemNamespace := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "kube-system"}}
	kubeclientset.CoreV1().Namespaces().Create(context.TODO(), kubeSystemNamespace, metav1.CreateOptions{})

	os.Exit(m.Run())
	<-stopCh
}

// Init syncs the test group
func (g *TestGroup) Init() {
	tenantObj := corev1alpha.Tenant{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Tenant",
			APIVersion: "apps.edgenet.io/v1alpha",
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
	g.tenantObj = tenantObj
}

func TestStartController(t *testing.T) {
	g := TestGroup{}
	g.Init()

	// Create a tenant
	tenantControllerTest := g.tenantObj.DeepCopy()
	tenantControllerTest.SetName("tenant-controller")

	edgenetclientset.CoreV1alpha().Tenants().Create(context.TODO(), tenantControllerTest, metav1.CreateOptions{})

	// Wait for the status update of the created object
	time.Sleep(250 * time.Millisecond)

	// Get the object and check the status
	tenant, err := edgenetclientset.CoreV1alpha().Tenants().Get(context.TODO(), tenantControllerTest.GetName(), metav1.GetOptions{})
	util.OK(t, err)

	tenant.Spec.Enabled = false
	edgenetclientset.CoreV1alpha().Tenants().Update(context.TODO(), tenant, metav1.UpdateOptions{})
	time.Sleep(250 * time.Millisecond)
	_, err = kubeclientset.RbacV1().Roles(tenant.GetName()).Get(context.TODO(), fmt.Sprintf("edgenet:tenant-owner-%s", tenant.Spec.Contact.Handle), metav1.GetOptions{})
	util.Equals(t, "roles.rbac.authorization.k8s.io \"edgenet:tenant-owner-johndoe\" not found", err.Error())
}

func TestCreate(t *testing.T) {
	g := TestGroup{}
	g.Init()

	tenant := g.tenantObj.DeepCopy()
	tenant.SetName("creation-test")

	edgenetclientset.CoreV1alpha().Tenants().Create(context.TODO(), tenant, metav1.CreateOptions{})
	time.Sleep(250 * time.Millisecond)
	t.Run("owner role configuration", func(t *testing.T) {
		tenant, err := edgenetclientset.CoreV1alpha().Tenants().Get(context.TODO(), tenant.GetName(), metav1.GetOptions{})
		util.OK(t, err)

		var acceptableUsePolicy *corev1alpha.AcceptableUsePolicy
		if acceptableUsePolicyRaw, err := edgenetclientset.CoreV1alpha().AcceptableUsePolicies().List(context.TODO(), metav1.ListOptions{}); err == nil {
			for _, acceptableUsePolicyRow := range acceptableUsePolicyRaw.Items {
				if acceptableUsePolicyRow.Spec.Email == tenant.Spec.Contact.Email {
					acceptableUsePolicy = acceptableUsePolicyRow.DeepCopy()

					acceptableUsePolicy.Spec.Accepted = true
					edgenetclientset.CoreV1alpha().AcceptableUsePolicies().Update(context.TODO(), acceptableUsePolicy, metav1.UpdateOptions{})
				}
			}
		}
		if acceptableUsePolicy == nil {
			t.Fail()
			return
		}
		if tenant.Status.PolicyAgreed == nil {
			tenant.Status.PolicyAgreed = make(map[string]bool)
		}
		tenant.Status.PolicyAgreed[acceptableUsePolicy.GetName()] = true
		edgenetclientset.CoreV1alpha().Tenants().UpdateStatus(context.TODO(), tenant, metav1.UpdateOptions{})
		time.Sleep(250 * time.Millisecond)

		t.Run("cluster role binding", func(t *testing.T) {
			_, err := kubeclientset.RbacV1().ClusterRoleBindings().Get(context.TODO(), fmt.Sprintf("edgenet:%s:tenants:%s-owner-%s", tenant.GetName(), tenant.GetName(), acceptableUsePolicy.GetName()), metav1.GetOptions{})
			util.OK(t, err)
		})
		t.Run("role binding", func(t *testing.T) {
			_, err := kubeclientset.RbacV1().RoleBindings(tenant.GetName()).Get(context.TODO(), "edgenet:tenant-owner", metav1.GetOptions{})
			util.OK(t, err)
		})
	})
	t.Run("cluster roles", func(t *testing.T) {
		_, err := kubeclientset.RbacV1().ClusterRoles().Get(context.TODO(), fmt.Sprintf("edgenet:%s:tenants:%s-owner", tenant.GetName(), tenant.GetName()), metav1.GetOptions{})
		util.OK(t, err)
	})
}
