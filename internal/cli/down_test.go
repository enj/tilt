package cli

import (
	"context"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/windmilleng/tilt/internal/analytics"
	"github.com/windmilleng/tilt/internal/dockercompose"
	"github.com/windmilleng/tilt/internal/k8s"
	"github.com/windmilleng/tilt/internal/k8s/testyaml"
	"github.com/windmilleng/tilt/internal/testutils"
	"github.com/windmilleng/tilt/internal/testutils/podbuilder"
	"github.com/windmilleng/tilt/internal/tiltfile"
	"github.com/windmilleng/tilt/pkg/model"
)

func TestDown(t *testing.T) {
	f := newDownFixture(t)
	defer f.TearDown()

	manifests := f.newK8sManifest()

	f.tfl.Result = tiltfile.TiltfileLoadResult{Manifests: manifests}
	err := f.cmd.down(f.ctx, f.deps, nil)
	assert.NoError(t, err)
	assert.Contains(t, f.kCli.DeletedYaml, "sancho")
}

func TestDownK8sFails(t *testing.T) {
	f := newDownFixture(t)
	defer f.TearDown()

	f.tfl.Result = tiltfile.TiltfileLoadResult{Manifests: f.newK8sManifest()}
	f.kCli.DeleteError = fmt.Errorf("GARBLEGARBLE")
	err := f.cmd.down(f.ctx, f.deps, nil)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "GARBLEGARBLE")
	}
}

func TestDownDCFails(t *testing.T) {
	f := newDownFixture(t)
	defer f.TearDown()

	f.tfl.Result = tiltfile.TiltfileLoadResult{Manifests: newDCManifest()}
	f.dcc.DownError = fmt.Errorf("GARBLEGARBLE")
	err := f.cmd.down(f.ctx, f.deps, nil)
	if assert.Error(t, err) {
		assert.Contains(t, err.Error(), "GARBLEGARBLE")
	}
}

func TestDownArgs(t *testing.T) {
	f := newDownFixture(t)
	defer f.TearDown()

	cmd := f.cmd.register()
	cmd.SetArgs([]string{"foo", "bar"})
	cmd.Run = func(cmd *cobra.Command, args []string) {
		ctx, _, _ := testutils.CtxAndAnalyticsForTest()
		err := f.cmd.run(ctx, args)
		require.NoError(t, err)
	}
	err := cmd.Execute()
	require.NoError(t, err)

	require.Equal(t, []string{"foo", "bar"}, f.tfl.PassedUserConfigState().Args)
}

func TestDownDoesNotDeleteStuffThatTiltDidntCreate(t *testing.T) {
	f := newDownFixture(t)
	defer f.TearDown()

	manifests := f.newK8sManifestWithoutManagedByTiltLabels()

	f.tfl.Result = tiltfile.TiltfileLoadResult{Manifests: manifests}
	err := f.cmd.down(f.ctx, f.deps, nil)
	assert.NoError(t, err)
	assert.NotContains(t, f.kCli.DeletedYaml, "sancho")
}

func (f *downFixture) newK8sManifest() []model.Manifest {
	manifests := []model.Manifest{model.Manifest{Name: "sancho"}.WithDeployTarget(model.K8sTarget{YAML: testyaml.SanchoYAML})}
	for _, m := range manifests {
		pb := podbuilder.New(f.t, m).WithPodLabel(k8s.ManagedByLabel, k8s.ManagedByValue)
		newEntities := []k8s.K8sEntity{}
		for _, e := range pb.ObjectTreeEntities() {
			newE, err := k8s.InjectLabels(e, []model.LabelPair{k8s.TiltManagedByLabel()})
			if err != nil {
				f.t.Fatal(err)
			}
			newEntities = append(newEntities, newE)
		}
		f.kCli.InjectEntityByName(newEntities...)
	}

	return manifests
}

func (f *downFixture) newK8sManifestWithoutManagedByTiltLabels() []model.Manifest {
	return []model.Manifest{model.Manifest{Name: "sancho"}.WithDeployTarget(model.K8sTarget{YAML: testyaml.SanchoYAML})}
}

func newDCManifest() []model.Manifest {
	return []model.Manifest{model.Manifest{Name: "fe"}.WithDeployTarget(model.DockerComposeTarget{
		Name:        "fe",
		ConfigPaths: []string{"dc.yaml"},
	})}
}

type downFixture struct {
	t      *testing.T
	ctx    context.Context
	cancel func()
	cmd    *downCmd
	deps   DownDeps
	tfl    *tiltfile.FakeTiltfileLoader
	dcc    *dockercompose.FakeDCClient
	kCli   *k8s.FakeK8sClient
}

func newDownFixture(t *testing.T) downFixture {
	ctx, _, _ := testutils.CtxAndAnalyticsForTest()
	ctx, cancel := context.WithCancel(ctx)
	tfl := tiltfile.NewFakeTiltfileLoader()
	dcc := dockercompose.NewFakeDockerComposeClient(t, ctx)
	kCli := k8s.NewFakeK8sClient()
	downDeps := DownDeps{tfl, dcc, kCli}
	cmd := &downCmd{downDepsProvider: func(ctx context.Context, tiltAnalytics *analytics.TiltAnalytics) (deps DownDeps, err error) {
		return downDeps, nil
	}}
	return downFixture{
		t:      t,
		ctx:    ctx,
		cancel: cancel,
		cmd:    cmd,
		deps:   downDeps,
		tfl:    tfl,
		dcc:    dcc,
		kCli:   kCli,
	}
}

func (f *downFixture) TearDown() {
	f.cancel()
}
