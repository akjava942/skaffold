/*
Copyright 2020 The Skaffold Authors

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

package docker

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/pkg/errors"

	"github.com/GoogleContainerTools/skaffold/pkg/skaffold/build"
	deploy "github.com/GoogleContainerTools/skaffold/pkg/skaffold/deploy/types"
	dockerutil "github.com/GoogleContainerTools/skaffold/pkg/skaffold/docker"
	"github.com/GoogleContainerTools/skaffold/pkg/skaffold/schema/latest"
	"github.com/GoogleContainerTools/skaffold/pkg/skaffold/util"
)

type Deployer struct {
	cfg                *latest.DockerDeploy
	client             dockerutil.LocalDaemon
	deployedContainers []string
	pf                 []*latest.PortForwardResource
	network            string
	once               sync.Once
}

type Config interface {
	deploy.Config
}

func NewDeployer(cfg Config, labels map[string]string, d *latest.DockerDeploy, resources []*latest.PortForwardResource) (*Deployer, error) {
	client, err := dockerutil.NewAPIClient(cfg)
	var pf []*latest.PortForwardResource
	for _, r := range resources {
		if r.Type == "Container" {
			pf = append(pf, r)
		}
	}
	if err != nil {
		return nil, err
	}
	return &Deployer{
		cfg:     d,
		client:  client,
		pf:      pf,
		network: "skaffold-network",
	}, nil
}

func (d *Deployer) Deploy(ctx context.Context, out io.Writer, builds []build.Artifact) ([]string, error) {
	var err error
	d.once.Do(func() {
		err = d.client.NetworkCreate(ctx, d.network)
	})
	if err != nil {
		return nil, fmt.Errorf("creating skaffold network %s: %w", d.network, err)
	}
	for _, b := range builds {
		if !util.StrSliceContains(d.cfg.Images, b.ImageName) {
			continue
		}
		var pf []*latest.PortForwardResource
		for _, r := range d.pf {
			if r.Name == b.ImageName {
				pf = append(pf, r)
			}
		}
		id, err := d.client.Run(ctx, out, b.ImageName, b.Tag, d.network, pf)
		if err != nil {
			return nil, errors.Wrap(err, "creating container in local docker")
		}
		fmt.Fprintf(os.Stdout, "container %s created from image %s\n", id, b.Tag)
		d.deployedContainers = append(d.deployedContainers, id)
	}

	return nil, nil
}

func (d *Deployer) Dependencies() ([]string, error) {
	// noop since there is no deploy config
	// TODO(nkubala): add docker-compose.yml here?
	return nil, nil
}

func (d *Deployer) Cleanup(ctx context.Context, out io.Writer) error {
	// stop, remove, prune?
	for _, id := range d.deployedContainers {
		if err := d.client.Delete(ctx, out, id); err != nil {
			return errors.Wrap(err, "cleaning up deployed container")
		}
	}

	err := d.client.NetworkRemove(ctx, d.network)
	return errors.Wrap(err, "cleaning up skaffold created network")
}

func (d *Deployer) Render(context.Context, io.Writer, []build.Artifact, bool, string) error {
	// TODO(nkubala): implement
	return errors.New("not implemented")
}