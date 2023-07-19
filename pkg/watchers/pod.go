package watchers

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"sync"

	"github.com/cilium/cilium/api/v1/models"
	"github.com/cilium/cilium/daemon/cmd"
	"github.com/cilium/cilium/pkg/datapath"
	"github.com/cilium/cilium/pkg/endpoint"
	"github.com/cilium/cilium/pkg/fqdn/restore"
	"github.com/cilium/cilium/pkg/identity"
	identityCache "github.com/cilium/cilium/pkg/identity/cache"
	"github.com/cilium/cilium/pkg/k8s"
	"github.com/cilium/cilium/pkg/k8s/client"
	"github.com/cilium/cilium/pkg/k8s/informer"
	slim_corev1 "github.com/cilium/cilium/pkg/k8s/slim/k8s/api/core/v1"
	slimclientset "github.com/cilium/cilium/pkg/k8s/slim/k8s/client/clientset/versioned"
	"github.com/cilium/cilium/pkg/k8s/utils"
	"github.com/cilium/cilium/pkg/labels"
	"github.com/cilium/cilium/pkg/labelsfilter"
	"github.com/cilium/cilium/pkg/lock"
	monitorAPI "github.com/cilium/cilium/pkg/monitor/api"
	"github.com/cilium/cilium/pkg/option"
	"github.com/cilium/cilium/pkg/policy"
	"github.com/cilium/cilium/pkg/types"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"k8s.io/client-go/tools/cache"
)

type OperatorOwner struct {
}

// QueueEndpointBuild puts the given endpoint in the processing queue

func (o *OperatorOwner) QueueEndpointBuild(ctx context.Context, epID uint64) (func(), error) {
	return nil, nil
}

// GetCompilationLock returns the mutex responsible for synchronizing compilation
// of BPF programs.
func (o *OperatorOwner) GetCompilationLock() *lock.RWMutex {
	return nil
}

// GetCIDRPrefixLengths returns the sorted list of unique prefix lengths used
// by CIDR policies.
func (o *OperatorOwner) GetCIDRPrefixLengths() (s6, s4 []int) {
	return nil, nil
}

// SendNotification is called to emit an agent notification

func (o *OperatorOwner) SendNotification(msg monitorAPI.AgentNotifyMessage) error {
	return nil
}

// Datapath returns a reference to the datapath implementation.
func (o *OperatorOwner) Datapath() datapath.Datapath {
	return nil
}

// GetDNSRules creates a fresh copy of DNS rules that can be used when
// endpoint is restored on a restart.
// The endpoint lock must not be held while calling this function.
func (o *OperatorOwner) GetDNSRules(epID uint16) restore.DNSRules {
	return nil
}

// RemoveRestoredDNSRules removes any restored DNS rules for
// this endpoint from the DNS proxy.
func (o *OperatorOwner) RemoveRestoredDNSRules(epID uint16) {

}

type identityAllocatorOwner struct{}

// UpdateIdentities will be called when identities have changed
func (m *identityAllocatorOwner) UpdateIdentities(added, deleted identityCache.IdentityCache) {}

// GetNodeSuffix must return the node specific suffix to use
func (m *identityAllocatorOwner) GetNodeSuffix() string {
	return "vm-allocator"
}

type policyRepoGetter struct {
}

func (prg *policyRepoGetter) GetPolicyRepository() *policy.Repository {
	return &policy.Repository{}

}

type namedPortsGetter struct {
}

func (npg *namedPortsGetter) GetNamedPorts() (npm types.NamedPortMultiMap) {
	return nil
}

type cachingIdentityAllocator struct {
	*identityCache.CachingIdentityAllocator
}

func NewCachingIdentityAllocator(owner identityCache.IdentityAllocatorOwner) cachingIdentityAllocator {
	return cachingIdentityAllocator{
		CachingIdentityAllocator: identityCache.NewCachingIdentityAllocator(owner),
	}
}

func (c cachingIdentityAllocator) AllocateCIDRsForIPs(ips []net.IP, newlyAllocatedIdentities map[netip.Prefix]*identity.Identity) ([]*identity.Identity, error) {
	return nil, nil
}

func (c cachingIdentityAllocator) ReleaseCIDRIdentitiesByID(ctx context.Context, identities []identity.NumericIdentity) {
}

// PodInit initializes the pod watcher
func PodInit(wg *sync.WaitGroup, clientset client.Clientset, slimClient slimclientset.Interface, ctx context.Context) {
	_, podInformer := informer.NewInformer(
		utils.ListerWatcherFromTyped[*slim_corev1.PodList](slimClient.CoreV1().Pods("")),
		&slim_corev1.Pod{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				pod := obj.(*slim_corev1.Pod)
				epTemplate := &models.EndpointChangeRequest{
					ContainerID:           string(pod.UID), // NOTE: cilium-cni takes CNI `CmdArgs.ContainerID` instead
					State:                 models.EndpointStateWaitingDashForDashIdentity.Pointer(),
					Addressing:            &models.AddressPair{},
					K8sPodName:            pod.Name,
					K8sNamespace:          pod.Namespace,
					DatapathConfiguration: &models.EndpointDatapathConfiguration{},
				}

				iao := &identityAllocatorOwner{}

				option.Config.IdentityAllocationMode = option.IdentityAllocationModeCRD
				identityAllocator := NewCachingIdentityAllocator(iao)
				identityAllocator.InitIdentityAllocator(clientset, nil)

				owner := &OperatorOwner{}
				policyGetter := &policyRepoGetter{}
				namedPortsGetter := &namedPortsGetter{}
				ep, err := endpoint.NewEndpointFromChangeModel(context.TODO(), owner, policyGetter, namedPortsGetter, nil, identityAllocator, epTemplate)
				if err != nil {
					fmt.Println("unable to parse endpoint parameters: %s", err)
				}
				fmt.Println("yyyyyyyy")
				fmt.Println(ep.OpLabels)
				labelsfilter.ParseLabelPrefixCfg([]string{}, "")

				ns, _ := slimClient.CoreV1().Namespaces().Get(context.TODO(), pod.Namespace, metav1.GetOptions{})
				addLabels := labels.NewLabelsFromModel(epTemplate.Labels)
				infoLabels := labels.NewLabelsFromModel([]string{})
				containerPorts, lbls, _, _ := k8s.GetPodMetadata(ns, pod)
				k8sLbls := labels.Map2Labels(lbls, labels.LabelSourceK8s)
				identityLabels, info := labelsfilter.Filter(k8sLbls)
				ep.SetPod(pod)
				fmt.Printf("container ports %#v", containerPorts)
				// pods without container ports will not has cilium endpoints created because
				// "Skipping CiliumEndpoint update because k8s metadata is not yet available"
				ep.SetK8sMetadata(containerPorts)
				addLabels.MergeLabels(identityLabels)
				infoLabels.MergeLabels(info)

				fmt.Println(ep)
				endpointManager := cmd.WithDefaultEndpointManager(context.TODO(), clientset, endpoint.CheckHealth)
				if err := endpointManager.AddEndpoint(owner, ep, "Create endpoint"); err != nil {
					fmt.Println("unable to add endpoint: %s", err)
				}

				ep.UpdateLabels(context.TODO(), identityLabels, labels.Labels(infoLabels), true)
			},
			UpdateFunc: func(_, newObj interface{}) {
				pod := newObj.(*slim_corev1.Pod)
				fmt.Println(pod.Name)
			},
			DeleteFunc: func(obj interface{}) {
				pod := obj.(*slim_corev1.Pod)
				fmt.Println(pod.Name)
			},
		},
		nil, // transfom function
	)

	wg.Add(1)
	go func() {
		defer wg.Done()
		podInformer.Run(ctx.Done())
	}()
}
