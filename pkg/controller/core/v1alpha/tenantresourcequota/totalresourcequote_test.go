package tenantresourcequota

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
	"github.com/EdgeNet-project/edgenet/pkg/util"
	"github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	testclient "k8s.io/client-go/kubernetes/fake"
)

// The main structure of test group
type TestGroup struct {
	TRQObj        corev1alpha.TenantResourceQuota
	claimObj      corev1alpha.TenantResourceDetails
	dropObj       corev1alpha.TenantResourceDetails
	tenantObj     corev1alpha.Tenant
	nodeObj       corev1.Node
	client        kubernetes.Interface
	edgenetClient versioned.Interface
	handler       Handler
}

func TestMain(m *testing.M) {
	flag.String("dir", "../../../..", "Override the directory.")
	flag.String("smtp-path", "../../../../configs/smtp_test.yaml", "Set SMTP path.")
	flag.Parse()

	log.SetOutput(ioutil.Discard)
	logrus.SetOutput(ioutil.Discard)
	os.Exit(m.Run())
}

func (g *TestGroup) Init() {
	TRQObj := corev1alpha.TenantResourceQuota{
		TypeMeta: metav1.TypeMeta{
			Kind:       "TenantResourceQuota",
			APIVersion: "apps.edgenet.io/v1alpha",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "edgenet",
			UID:  "trq",
		},
	}
	claimObj := corev1alpha.TenantResourceDetails{
		Name:   "Default",
		CPU:    "12000m",
		Memory: "12Gi",
	}
	dropObj := corev1alpha.TenantResourceDetails{
		Name:   "Default",
		CPU:    "10000m",
		Memory: "10Gi",
	}
	tenantObj := corev1alpha.Tenant{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Tenant",
			APIVersion: "apps.edgenet.io/v1alpha",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "edgenet",
			UID:  "edgenet",
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
				Username:  "johndoe",
			},
			Enabled: true,
		},
	}
	nodeObj := corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "edgenet",
			OwnerReferences: []metav1.OwnerReference{
				metav1.OwnerReference{
					APIVersion: "apps.edgenet.io/v1alpha",
					Kind:       "Tenant",
					Name:       "edgenet",
					UID:        "edgenet"},
			},
		},
		TypeMeta: metav1.TypeMeta{
			Kind:       "Node",
			APIVersion: "v1",
		},
		Status: corev1.NodeStatus{
			Capacity: corev1.ResourceList{
				corev1.ResourceMemory:           resource.MustParse("4Gi"),
				corev1.ResourceCPU:              resource.MustParse("2"),
				corev1.ResourceEphemeralStorage: resource.MustParse("51493088"),
				corev1.ResourcePods:             resource.MustParse("100"),
			},
			Allocatable: corev1.ResourceList{
				corev1.ResourceMemory:           resource.MustParse("4Gi"),
				corev1.ResourceCPU:              resource.MustParse("2"),
				corev1.ResourceEphemeralStorage: resource.MustParse("51493088"),
				corev1.ResourcePods:             resource.MustParse("100"),
			},
			Conditions: []corev1.NodeCondition{
				corev1.NodeCondition{
					Type:   "Ready",
					Status: "True",
				},
			},
		},
	}
	g.TRQObj = TRQObj
	g.claimObj = claimObj
	g.dropObj = dropObj
	g.tenantObj = tenantObj
	g.nodeObj = nodeObj
	g.client = testclient.NewSimpleClientset()
	g.edgenetClient = edgenettestclient.NewSimpleClientset()
	// Imitate tenant creation processes
	g.edgenetClient.CoreV1alpha().Tenants().Create(context.TODO(), g.tenantObj.DeepCopy(), metav1.CreateOptions{})
	namespace := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("%s", g.tenantObj.GetName())}}
	namespaceLabels := map[string]string{"owner": "tenant", "owner-name": g.tenantObj.GetName(), "tenant-name": g.tenantObj.GetName()}
	namespace.SetLabels(namespaceLabels)
	g.client.CoreV1().Namespaces().Create(context.TODO(), &namespace, metav1.CreateOptions{})
}

func TestHandlerInit(t *testing.T) {
	// Sync the test group
	g := TestGroup{}
	g.Init()
	// Initialize the handler
	g.handler.Init(g.client, g.edgenetClient)
	util.Equals(t, g.client, g.handler.clientset)
	util.Equals(t, g.edgenetClient, g.handler.edgenetClientset)
}

func TestCreate(t *testing.T) {
	g := TestGroup{}
	g.Init()
	g.handler.Init(g.client, g.edgenetClient)

	cases := map[string]struct {
		input    []time.Duration
		sleep    time.Duration
		expected int
	}{
		"without expiry date": {nil, 110, 2},
		"expiries soon":       {[]time.Duration{100}, 110, 0},
		"expired":             {[]time.Duration{-100}, 0, 0},
		"mix/1":               {[]time.Duration{100, 1000, -100}, 0, 4},
		"mix/2":               {[]time.Duration{100, 1000, -100}, 110, 2},
		"mix/3":               {[]time.Duration{100, 50, 1000, 1400, -10, -100}, 0, 8},
		"mix/4":               {[]time.Duration{100, 50, 1000, 1400, -10, -100}, 110, 4},
	}
	for k, tc := range cases {
		t.Run(k, func(t *testing.T) {
			TRQ := g.TRQObj
			TRQ.SetUID(types.UID(k))
			claim := g.claimObj
			drop := g.dropObj
			if tc.input != nil {
				for _, input := range tc.input {
					claim.Expiry = &metav1.Time{
						Time: time.Now().Add(input * time.Millisecond),
					}
					TRQ.Spec.Claim = append(TRQ.Spec.Claim, claim)
					drop.Expiry = &metav1.Time{
						Time: time.Now().Add(input * time.Millisecond),
					}
					TRQ.Spec.Drop = append(TRQ.Spec.Drop, drop)
				}
			} else {
				TRQ.Spec.Claim = append(TRQ.Spec.Claim, claim)
				TRQ.Spec.Drop = append(TRQ.Spec.Drop, drop)
			}
			g.edgenetClient.CoreV1alpha().TenantResourceQuotas().Create(context.TODO(), TRQ.DeepCopy(), metav1.CreateOptions{})
			g.handler.ObjectCreated(TRQ.DeepCopy())
			defer g.edgenetClient.CoreV1alpha().TenantResourceQuotas().Delete(context.TODO(), TRQ.GetName(), metav1.DeleteOptions{})
			time.Sleep(tc.sleep * time.Millisecond)
			TRQCopy, err := g.edgenetClient.CoreV1alpha().TenantResourceQuotas().Get(context.TODO(), TRQ.GetName(), metav1.GetOptions{})
			util.OK(t, err)
			util.Equals(t, tc.expected, (len(TRQCopy.Spec.Claim) + len(TRQCopy.Spec.Drop)))
		})
	}

	t.Run("exceeded", func(t *testing.T) {
		/*g.edgenetClient.AppsV1alpha().Slices(g.sliceObj.GetNamespace()).Create(context.TODO(), g.sliceObj.DeepCopy(), metav1.CreateOptions{})
		childNamespace := fmt.Sprintf("%s-slice-%s", g.sliceObj.GetNamespace(), g.sliceObj.GetName())
		namespace := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: childNamespace}}
		namespaceLabels := map[string]string{"owner": "slice", "owner-name": g.sliceObj.GetName(), "tenant-name": g.tenantObj.GetName()}
		namespace.SetLabels(namespaceLabels)
		g.client.CoreV1().Namespaces().Create(context.TODO(), &namespace, metav1.CreateOptions{})
		quota := corev1.ResourceQuota{
			ObjectMeta: metav1.ObjectMeta{
				Name: "slice-high-quota",
			},
			Spec: corev1.ResourceQuotaSpec{
				Hard: map[corev1.ResourceName]resource.Quantity{
					"cpu":              resource.MustParse("8000m"),
					"memory":           resource.MustParse("8192Mi"),
					"requests.storage": resource.MustParse("8Gi"),
				},
			},
		}
		g.client.CoreV1().ResourceQuotas(childNamespace).Create(context.TODO(), quota.DeepCopy(), metav1.CreateOptions{})

		TRQ := g.TRQObj
		TRQ.Status.Exceeded = true
		g.edgenetClient.CoreV1alpha().TenantResourceQuotas().Create(context.TODO(), TRQ.DeepCopy(), metav1.CreateOptions{})
		g.handler.ObjectCreated(TRQ.DeepCopy())
		defer g.edgenetClient.CoreV1alpha().TenantResourceQuotas().Delete(context.TODO(), TRQ.GetName(), metav1.DeleteOptions{})
		TRQCopy, err := g.edgenetClient.CoreV1alpha().TenantResourceQuotas().Get(context.TODO(), TRQ.GetName(), metav1.GetOptions{})
		util.OK(t, err)
		util.Equals(t, true, TRQCopy.Spec.Enabled)

		_, err = g.edgenetClient.AppsV1alpha().Slices(g.sliceObj.GetNamespace()).Get(context.TODO(), g.sliceObj.GetName(), metav1.GetOptions{})
		util.Equals(t, true, errors.IsNotFound(err))

		TRQCopy, err = g.edgenetClient.CoreV1alpha().TenantResourceQuotas().Get(context.TODO(), TRQ.GetName(), metav1.GetOptions{})
		util.OK(t, err)
		util.Equals(t, false, TRQCopy.Status.Exceeded)
		*/
	})
}

func TestUpdate(t *testing.T) {
	g := TestGroup{}
	g.Init()
	g.handler.Init(g.client, g.edgenetClient)
	TRQ := g.TRQObj
	_, err := g.edgenetClient.CoreV1alpha().TenantResourceQuotas().Create(context.TODO(), TRQ.DeepCopy(), metav1.CreateOptions{})
	util.OK(t, err)
	g.handler.ObjectCreated(TRQ.DeepCopy())
	defer g.edgenetClient.CoreV1alpha().TenantResourceQuotas().Delete(context.TODO(), TRQ.GetName(), metav1.DeleteOptions{})

	cases := map[string]struct {
		input    []time.Duration
		sleep    time.Duration
		expected int
	}{
		"without expiry date": {nil, 30, 2},
		"expiries soon":       {[]time.Duration{30}, 200, 0},
		"expired":             {[]time.Duration{-100}, 0, 0},
		"mix/1":               {[]time.Duration{30, 500, -100}, 0, 4},
		"mix/2":               {[]time.Duration{30, 500, -100}, 200, 2},
		"mix/3":               {[]time.Duration{30, 10, 500, 800, -10, -100}, 0, 8},
		"mix/4":               {[]time.Duration{30, 10, 500, 800, -10, -100}, 200, 4},
	}
	for k, tc := range cases {
		t.Run(k, func(t *testing.T) {
			TRQCopy, err := g.edgenetClient.CoreV1alpha().TenantResourceQuotas().Get(context.TODO(), TRQ.GetName(), metav1.GetOptions{})
			util.OK(t, err)
			TRQCopy.Spec.Claim = []corev1alpha.TenantResourceDetails{}
			TRQCopy.Spec.Drop = []corev1alpha.TenantResourceDetails{}

			var field fields
			field.spec = true
			claim := g.claimObj
			drop := g.dropObj
			if tc.input != nil {
				for _, expiry := range tc.input {
					claim.Expiry = &metav1.Time{
						Time: time.Now().Add(expiry * time.Millisecond),
					}
					TRQCopy.Spec.Claim = append(TRQCopy.Spec.Claim, claim)
					drop.Expiry = &metav1.Time{
						Time: time.Now().Add(expiry * time.Millisecond),
					}
					TRQCopy.Spec.Drop = append(TRQCopy.Spec.Drop, drop)
				}
				field.expiry = true
			} else {
				TRQCopy.Spec.Claim = append(TRQCopy.Spec.Claim, claim)
				TRQCopy.Spec.Drop = append(TRQCopy.Spec.Drop, drop)
				field.expiry = false
			}
			_, err = g.edgenetClient.CoreV1alpha().TenantResourceQuotas().Update(context.TODO(), TRQCopy.DeepCopy(), metav1.UpdateOptions{})
			util.OK(t, err)
			g.handler.ObjectUpdated(TRQCopy.DeepCopy(), field)
			time.Sleep(tc.sleep * time.Millisecond)
			TRQCopy, err = g.edgenetClient.CoreV1alpha().TenantResourceQuotas().Get(context.TODO(), TRQCopy.GetName(), metav1.GetOptions{})
			util.OK(t, err)
			util.Equals(t, tc.expected, (len(TRQCopy.Spec.Claim) + len(TRQCopy.Spec.Drop)))
		})
		time.Sleep(500 * time.Millisecond)
	}
	t.Run("total quota", func(t *testing.T) {
		/*g.edgenetClient.AppsV1alpha().Teams(g.teamObj.GetNamespace()).Create(context.TODO(), g.teamObj.DeepCopy(), metav1.CreateOptions{})
		teamChildNamespace := fmt.Sprintf("%s-team-%s", g.teamObj.GetNamespace(), g.teamObj.GetName())
		namespace := corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: teamChildNamespace}}
		namespaceLabels := map[string]string{"owner": "team", "owner-name": g.teamObj.GetName(), "tenant-name": g.tenantObj.GetName()}
		namespace.SetLabels(namespaceLabels)
		g.client.CoreV1().Namespaces().Create(context.TODO(), &namespace, metav1.CreateOptions{})
		defer g.client.CoreV1().Namespaces().Delete(context.TODO(), namespace.GetName(), metav1.DeleteOptions{})

		TRQCopy, _ := g.edgenetClient.CoreV1alpha().TenantResourceQuotas().Get(context.TODO(), TRQ.GetName(), metav1.GetOptions{})
		TRQCopy.Spec.Claim = []corev1alpha.TenantResourceDetails{}
		TRQCopy.Spec.Drop = []corev1alpha.TenantResourceDetails{}
		_, err = g.edgenetClient.CoreV1alpha().TenantResourceQuotas().Update(context.TODO(), TRQCopy.DeepCopy(), metav1.UpdateOptions{})
		util.OK(t, err)
		var field fields
		field.spec = true
		g.handler.ObjectUpdated(TRQCopy.DeepCopy(), field)
		time.Sleep(100 * time.Millisecond)

		cases := map[string]struct {
			input    []corev1alpha.TenantResourceDetails
			expiry   []time.Duration
			kind     []string
			quota    string
			expected bool
		}{
			"claim/high":                                  {[]corev1alpha.TenantResourceDetails{g.claimObj}, nil, []string{"Claim"}, "High", false},
			"claim expires soon/high":                     {[]corev1alpha.TenantResourceDetails{g.claimObj}, []time.Duration{50}, []string{"Claim"}, "High", true},
			"claim-drop/low":                              {[]corev1alpha.TenantResourceDetails{g.claimObj, g.dropObj}, nil, []string{"Claim", "Drop"}, "Low", false},
			"claim-drop/high":                             {[]corev1alpha.TenantResourceDetails{g.claimObj, g.dropObj}, nil, []string{"Claim", "Drop"}, "High", true},
			"claim-drop expires soon/high":                {[]corev1alpha.TenantResourceDetails{g.claimObj, g.dropObj}, []time.Duration{800, 80}, []string{"Claim", "Drop"}, "High", true},
			"claim-claim and then drop expires soon/high": {[]corev1alpha.TenantResourceDetails{g.claimObj, g.claimObj, g.dropObj}, []time.Duration{800, 50, 90}, []string{"Claim", "Claim", "Drop"}, "High", false},
			"drop-claim and then drop expires soon/high":  {[]corev1alpha.TenantResourceDetails{g.dropObj, g.claimObj, g.dropObj}, []time.Duration{800, 50, 90}, []string{"Drop", "Claim", "Drop"}, "High", true},
		}
		for k, tc := range cases {
			t.Run(k, func(t *testing.T) {
				TRQCopy, err = g.edgenetClient.CoreV1alpha().TenantResourceQuotas().Get(context.TODO(), TRQ.GetName(), metav1.GetOptions{})
				TRQCopy.Spec.Claim = []corev1alpha.TenantResourceDetails{}
				TRQCopy.Spec.Drop = []corev1alpha.TenantResourceDetails{}

				slice := g.sliceObj
				slice.SetName(k)
				slice.SetNamespace(teamChildNamespace)
				g.edgenetClient.AppsV1alpha().Slices(teamChildNamespace).Create(context.TODO(), slice.DeepCopy(), metav1.CreateOptions{})
				defer g.edgenetClient.AppsV1alpha().Slices(teamChildNamespace).Delete(context.TODO(), slice.GetName(), metav1.DeleteOptions{})
				childNamespace := fmt.Sprintf("%s-slice-%s", teamChildNamespace, slice.GetName())
				namespace = corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: childNamespace}}
				namespaceLabels = map[string]string{"owner": "slice", "owner-name": slice.GetName(), "tenant-name": g.tenantObj.GetName()}
				namespace.SetLabels(namespaceLabels)
				g.client.CoreV1().Namespaces().Create(context.TODO(), &namespace, metav1.CreateOptions{})
				defer g.client.CoreV1().Namespaces().Delete(context.TODO(), namespace.GetName(), metav1.DeleteOptions{})

				var quota corev1.ResourceQuota
				if tc.quota == "High" {
					quota = corev1.ResourceQuota{
						ObjectMeta: metav1.ObjectMeta{
							Name: "slice-high-quota",
						},
						Spec: corev1.ResourceQuotaSpec{
							Hard: map[corev1.ResourceName]resource.Quantity{
								"cpu":              resource.MustParse("8000m"),
								"memory":           resource.MustParse("8192Mi"),
								"requests.storage": resource.MustParse("8Gi"),
							},
						},
					}
				} else if tc.quota == "Low" {
					quota = corev1.ResourceQuota{
						ObjectMeta: metav1.ObjectMeta{
							Name: "slice-low-quota",
						},
						Spec: corev1.ResourceQuotaSpec{
							Hard: map[corev1.ResourceName]resource.Quantity{
								"cpu":              resource.MustParse("2000m"),
								"memory":           resource.MustParse("2048Mi"),
								"requests.storage": resource.MustParse("500Mi"),
							},
						},
					}
				}
				g.client.CoreV1().ResourceQuotas(childNamespace).Create(context.TODO(), quota.DeepCopy(), metav1.CreateOptions{})
				defer g.client.CoreV1().ResourceQuotas(childNamespace).Delete(context.TODO(), quota.GetName(), metav1.DeleteOptions{})

				var field fields
				field.spec = true
				for i, expiry := range tc.input {
					if tc.kind[i] == "Claim" {
						claim := expiry
						if tc.expiry != nil {
							claim.Expiry = &metav1.Time{
								Time: time.Now().Add(tc.expiry[i] * time.Millisecond),
							}
							field.expiry = true
						} else {
							field.expiry = false
						}
						TRQCopy.Spec.Claim = append(TRQCopy.Spec.Claim, claim)
					} else if tc.kind[i] == "Drop" {
						drop := expiry
						if tc.expiry != nil {
							drop.Expiry = &metav1.Time{
								Time: time.Now().Add(tc.expiry[i] * time.Millisecond),
							}
							field.expiry = true
						} else {
							field.expiry = false
						}
						TRQCopy.Spec.Drop = append(TRQCopy.Spec.Drop, drop)
					}
				}
				_, err = g.edgenetClient.CoreV1alpha().TenantResourceQuotas().Update(context.TODO(), TRQCopy.DeepCopy(), metav1.UpdateOptions{})
				util.OK(t, err)
				g.handler.ObjectUpdated(TRQCopy.DeepCopy(), field)
				time.Sleep(150 * time.Millisecond)

				_, err = g.edgenetClient.AppsV1alpha().Slices(teamChildNamespace).Get(context.TODO(), slice.GetName(), metav1.GetOptions{})
				util.Equals(t, tc.expected, errors.IsNotFound(err))
			})
			time.Sleep(500 * time.Millisecond)
		}*/
	})
}

func TestCreateTenantResourceQuota(t *testing.T) {
	g := TestGroup{}
	g.Init()
	g.handler.Init(g.client, g.edgenetClient)

	_, err := g.edgenetClient.CoreV1alpha().TenantResourceQuotas().Get(context.TODO(), g.TRQObj.GetName(), metav1.GetOptions{})
	util.Equals(t, true, errors.IsNotFound(err))
	g.handler.Create(g.TRQObj.GetName())
	_, err = g.edgenetClient.CoreV1alpha().TenantResourceQuotas().Get(context.TODO(), g.TRQObj.GetName(), metav1.GetOptions{})
	util.OK(t, err)
}
