package commands

import (
	"context"
	"github.com/google/uuid"
	"github.com/kluctl/kluctl/v2/pkg/deployment"
	utils2 "github.com/kluctl/kluctl/v2/pkg/deployment/utils"
	"github.com/kluctl/kluctl/v2/pkg/k8s"
	k8s2 "github.com/kluctl/kluctl/v2/pkg/types/k8s"
	"github.com/kluctl/kluctl/v2/pkg/types/result"
	"github.com/kluctl/kluctl/v2/pkg/validation"
)

type ValidateCommand struct {
	c             *deployment.DeploymentCollection
	discriminator string

	dew *utils2.DeploymentErrorsAndWarnings
	ru  *utils2.RemoteObjectUtils
}

func NewValidateCommand(ctx context.Context, discriminator string, c *deployment.DeploymentCollection) *ValidateCommand {
	cmd := &ValidateCommand{
		c:             c,
		discriminator: discriminator,
		dew:           utils2.NewDeploymentErrorsAndWarnings(),
	}
	cmd.ru = utils2.NewRemoteObjectsUtil(ctx, cmd.dew)
	return cmd
}

func (cmd *ValidateCommand) Run(ctx context.Context, k *k8s.K8sCluster) (*result.ValidateResult, error) {
	ret := result.ValidateResult{
		Id:    uuid.New().String(),
		Ready: true,
	}

	cmd.dew.Init()

	err := cmd.ru.UpdateRemoteObjects(k, &cmd.discriminator, cmd.c.LocalObjectRefs(), true)
	if err != nil {
		return nil, err
	}

	ad := utils2.NewApplyDeploymentsUtil(ctx, cmd.dew, cmd.c.Deployments, cmd.ru, k, &utils2.ApplyUtilOptions{})
	for _, d := range cmd.c.Deployments {
		au := ad.NewApplyUtil(ctx, nil)
		h := utils2.NewHooksUtil(au)
		for _, o := range d.Objects {
			hook := h.GetHook(o)
			if hook != nil && !hook.IsPersistent() {
				continue
			}

			ref := o.GetK8sRef()

			remoteObject := cmd.ru.GetRemoteObject(ref)
			if remoteObject == nil {
				ret.Errors = append(ret.Errors, result.DeploymentError{Ref: ref, Message: "object not found"})
				continue
			}
			r := validation.ValidateObject(k, remoteObject, true, false)
			if !r.Ready {
				ret.Ready = false
			}
			ret.Errors = append(ret.Errors, r.Errors...)
			ret.Warnings = append(ret.Warnings, r.Warnings...)
			ret.Results = append(ret.Results, r.Results...)
		}
	}

	ret.Warnings = append(ret.Warnings, cmd.dew.GetWarningsList()...)
	ret.Errors = append(ret.Errors, cmd.dew.GetErrorsList()...)

	return &ret, nil
}

func (cmd *ValidateCommand) ForgetRemoteObject(ref k8s2.ObjectRef) {
	cmd.ru.ForgetRemoteObject(ref)
}
