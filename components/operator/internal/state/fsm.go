package state

import (
	"context"
	"fmt"
	"reflect"
	"runtime"
	"strings"

	"github.com/kyma-project/docker-registry/components/operator/api/v1alpha1"
	"github.com/kyma-project/docker-registry/components/operator/internal/chart"
	"github.com/kyma-project/docker-registry/components/operator/internal/registry"
	"github.com/kyma-project/docker-registry/components/operator/internal/warning"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

var (
	defaultResult  = ctrl.Result{}
	secretCacheKey = types.NamespacedName{
		Name:      "dockerregistry-manifest-cache",
		Namespace: "kyma-system",
	}
)

type stateFn func(context.Context, *reconciler, *systemState) (stateFn, *ctrl.Result, error)

type cfg struct {
	finalizer     string
	chartPath     string
	managerPodUID string
}

type systemState struct {
	instance            v1alpha1.DockerRegistry
	statusSnapshot      v1alpha1.DockerRegistryStatus
	chartConfig         *chart.Config
	warningBuilder      *warning.Builder
	flagsBuilder        chart.FlagsBuilder
	nodePortResolver    *registry.NodePortResolver
	gatewayHostResolver registry.ExternalAccessResolver
}

func (s *systemState) saveStatusSnapshot() {
	result := s.instance.Status.DeepCopy()
	if result == nil {
		result = &v1alpha1.DockerRegistryStatus{}
	}
	s.statusSnapshot = *result
}

func (s *systemState) setState(state v1alpha1.State) {
	s.instance.Status.State = state
}

func (s *systemState) setServed(served v1alpha1.Served) {
	s.instance.Status.Served = served
}

func chartConfig(ctx context.Context, r *reconciler, namespace string) *chart.Config {
	return &chart.Config{
		Ctx:        ctx,
		Log:        r.log,
		Cache:      r.cache,
		CacheKey:   secretCacheKey,
		ManagerUID: r.managerPodUID,
		Cluster: chart.Cluster{
			Client: r.client,
			Config: r.config,
		},
		Release: chart.Release{
			ChartPath: r.chartPath,
			Namespace: namespace,
			Name:      "dockerregistry",
		},
	}
}

type k8s struct {
	client client.Client
	config *rest.Config
	record.EventRecorder
}

type reconciler struct {
	fn     stateFn
	log    *zap.SugaredLogger
	cache  chart.ManifestCache
	result ctrl.Result
	k8s
	cfg
}

func (m *reconciler) stateFnName() string {
	fullName := runtime.FuncForPC(reflect.ValueOf(m.fn).Pointer()).Name()
	splitFullName := strings.Split(fullName, ".")

	if len(splitFullName) < 3 {
		return fullName
	}

	shortName := splitFullName[2]
	return shortName
}

func (m *reconciler) Reconcile(ctx context.Context, v v1alpha1.DockerRegistry) (ctrl.Result, error) {
	state := systemState{
		instance:         v,
		warningBuilder:   warning.NewBuilder(),
		flagsBuilder:     chart.NewFlagsBuilder(),
		chartConfig:      chartConfig(ctx, m, v.Namespace),
		nodePortResolver: registry.NewNodePortResolver(registry.RandomNodePort),
		gatewayHostResolver: registry.NewExternalAccessResolver(
			fmt.Sprintf("registry-%s-%s", v.GetName(), v.GetNamespace()),
		),
	}
	state.saveStatusSnapshot()
	var err error
	var result *ctrl.Result
loop:
	for m.fn != nil && err == nil {
		select {
		case <-ctx.Done():
			err = ctx.Err()
			break loop

		default:
			m.log.Info(fmt.Sprintf("switching state: %s", m.stateFnName()))
			m.fn, result, err = m.fn(ctx, m, &state)
			if updateErr := updateDockerRegistryStatus(ctx, m, &state); updateErr != nil {
				err = updateErr
			}
		}
	}

	if result == nil {
		result = &defaultResult
	}

	m.log.
		With("error", err).
		With("result", result).
		Info("reconciliation done")

	return *result, err
}
