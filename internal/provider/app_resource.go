package provider

import (
	"context"
	"dov.dev/fly/fly-provider/graphql"
	"dov.dev/fly/fly-provider/internal/provider/modifiers"
	"dov.dev/fly/fly-provider/internal/utils"
	"errors"
	"fmt"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/tfsdk"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-go/tftypes"
	"github.com/hashicorp/terraform-plugin-log/tflog"
	"github.com/vektah/gqlparser/v2/gqlerror"
)

// Ensure provider defined types fully satisfy framework interfaces
var _ tfsdk.ResourceType = flyAppResourceType{}
var _ tfsdk.Resource = flyAppResource{}
var _ tfsdk.ResourceWithImportState = flyAppResource{}

type flyAppResourceType struct{}

type flyAppResourceData struct {
	Name            types.String   `tfsdk:"name"`
	Id              types.String   `tfsdk:"id"`
	Network         types.String   `tfsdk:"network"`
	Org             types.String   `tfsdk:"org"`
	PreferredRegion types.String   `tfsdk:"preferred_region"'`
	Regions         []types.String `tfsdk:"regions"`
}

func (ar flyAppResourceType) GetSchema(context.Context) (tfsdk.Schema, diag.Diagnostics) {
	return tfsdk.Schema{
		// This description is used by the documentation generator and the language server.
		MarkdownDescription: "Fly app resource",

		Attributes: map[string]tfsdk.Attribute{
			"name": {
				MarkdownDescription: "Name of application",
				Required:            true,
				Type:                types.StringType,
			},
			"id": {
				MarkdownDescription: "Name of application",
				Optional:            true,
				Computed:            true,
				Type:                types.StringType,
			},
			"network": {
				Computed:            true,
				Optional:            true,
				MarkdownDescription: "Optional custom network ID",
				PlanModifiers: tfsdk.AttributePlanModifiers{
					modifiers.StringDefault(""),
				},
				Type: types.StringType,
			},
			"org": {
				Computed:            true,
				Optional:            true,
				MarkdownDescription: "Optional org ID to operate upon",
				Type:                types.StringType,
			},
			"preferred_region": {
				Computed:            true,
				Optional:            true,
				MarkdownDescription: "Optional region to set as preferred",
				PlanModifiers: tfsdk.AttributePlanModifiers{
					modifiers.StringDefault(""),
				},
				Type: types.StringType,
			},
			"regions": {
				Computed:            true,
				Optional:            true,
				MarkdownDescription: "Optional list of regions to set in autoscaling config",
				Type:                types.ListType{ElemType: types.StringType},
			},
		},
	}, nil
}

func (ar flyAppResourceType) NewResource(_ context.Context, in tfsdk.Provider) (tfsdk.Resource, diag.Diagnostics) {
	provider, diags := convertProviderType(in)

	return flyAppResource{
		provider: provider,
	}, diags
}

type flyAppResource struct {
	provider provider
}

func (r flyAppResource) Create(ctx context.Context, req tfsdk.CreateResourceRequest, resp *tfsdk.CreateResourceResponse) {
	var data flyAppResourceData

	diags := req.Plan.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	if data.Org.Unknown {
		defaultOrg, err := utils.GetDefaultOrg(*r.provider.client)
		if err != nil {
			resp.Diagnostics.AddError("Could not detect default organization", err.Error())
			return
		}
		data.Org.Value = defaultOrg.Id
	}

	if len(data.Regions) > 0 {
		var rawRegions []graphql.AutoscaleRegionConfigInput
		var regions []types.String

		for _, s := range data.Regions {
			rawRegions = append(rawRegions, graphql.AutoscaleRegionConfigInput{
				Code: s.Value,
			})
		}

		mresp, err := graphql.CreateAppMutationWithAutoscaleConfig(context.Background(), *r.provider.client, data.Name.Value, data.Name.Value, data.Org.Value, data.PreferredRegion.Value, data.Network.Value, rawRegions)
		if err != nil {
			resp.Diagnostics.AddError("Create app failed (creating with autoscale config)", err.Error())
			return
		}

		for _, s := range mresp.UpdateAutoscaleConfig.App.Autoscaling.Regions {
			regions = append(regions, types.String{Value: s.Code})
		}

		data = flyAppResourceData{
			Id:              types.String{Value: mresp.CreateApp.App.Name},
			Network:         types.String{Value: mresp.CreateApp.App.Network},
			Org:             types.String{Value: mresp.CreateApp.App.Organization.Id},
			Name:            types.String{Value: mresp.CreateApp.App.Name},
			PreferredRegion: types.String{Value: mresp.CreateApp.App.Autoscaling.PreferredRegion},
			Regions:         regions,
		}
	} else {
		mresp, err := graphql.CreateAppMutation(context.Background(), *r.provider.client, data.Name.Value, data.Org.Value, data.PreferredRegion.Value, data.Network.Value)
		if err != nil {
			resp.Diagnostics.AddError("Create app failed", err.Error())
			return
		}
		data = flyAppResourceData{
			Id:              types.String{Value: mresp.CreateApp.App.Name},
			Network:         types.String{Value: mresp.CreateApp.App.Network},
			Org:             types.String{Value: mresp.CreateApp.App.Organization.Id},
			Name:            types.String{Value: mresp.CreateApp.App.Name},
			PreferredRegion: types.String{Value: mresp.CreateApp.App.Autoscaling.PreferredRegion},
		}
	}

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r flyAppResource) Read(ctx context.Context, req tfsdk.ReadResourceRequest, resp *tfsdk.ReadResourceResponse) {
	var data flyAppResourceData

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var appKey string
	if !data.Name.Unknown {
		appKey = data.Id.Value
	} else {
		appKey = data.Name.Value
	}

	query, err := graphql.GetFullApp(context.Background(), *r.provider.client, appKey)
	var errList gqlerror.List
	if errors.As(err, &errList) {
		for _, err := range errList {
			if err.Message == "Could not resolve " {
				return
			}
			resp.Diagnostics.AddError(err.Message, err.Path.String())
		}
	} else if err != nil {
		resp.Diagnostics.AddError("Read: query failed", err.Error())
	}

	var regions []types.String
	for _, s := range query.App.Autoscaling.Regions {
		regions = append(regions, types.String{Value: s.Code})
	}

	data = flyAppResourceData{
		Name:            types.String{Value: query.App.Name},
		Id:              types.String{Value: query.App.Name},
		Network:         types.String{Value: query.App.Network},
		Org:             types.String{Value: query.App.Organization.Id},
		PreferredRegion: types.String{Value: query.App.Autoscaling.PreferredRegion},
		Regions:         regions,
	}

	diags = resp.State.Set(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
}

func (r flyAppResource) Update(ctx context.Context, req tfsdk.UpdateResourceRequest, resp *tfsdk.UpdateResourceResponse) {
	var plan flyAppResourceData

	diags := req.Plan.Get(ctx, &plan)
	resp.Diagnostics.Append(diags...)

	if resp.Diagnostics.HasError() {
		return
	}

	var state flyAppResourceData
	diags = resp.State.Get(ctx, &state)
	resp.Diagnostics.Append(diags...)

	tflog.Info(ctx, fmt.Sprintf("existing: %+v, new: %+v", state, plan))

	if !plan.Org.Unknown && plan.Org.Value != state.Org.Value {
		resp.Diagnostics.AddError("Can't mutate org of existing app", "Can't swith org"+state.Org.Value+" to "+plan.Org.Value)
	}
	if !plan.PreferredRegion.Unknown && plan.PreferredRegion.Value != state.PreferredRegion.Value {
		resp.Diagnostics.AddError("Can't mutate PreferredRegion of existing app", "Can't switch preferred region "+state.PreferredRegion.Value+" to "+plan.PreferredRegion.Value)
	}
	if !plan.Name.Unknown && plan.Name.Value != state.Name.Value {
		resp.Diagnostics.AddError("Can't mutate Name of existing app", "Can't switch name "+state.Name.Value+" to "+plan.Name.Value)
	}
	if !plan.Network.Unknown && plan.Network.Value != state.Name.Value {
		resp.Diagnostics.AddError("Can't mutate network of existing app", "Can't switch network"+state.Network.Value+" to "+plan.Network.Value)
	}

	if len(plan.Regions) > 0 {
		state.Regions = plan.Regions

		var rawRegions []graphql.AutoscaleRegionConfigInput

		for _, s := range plan.Regions {
			rawRegions = append(rawRegions, graphql.AutoscaleRegionConfigInput{
				Code: s.Value,
			})
		}

		_, err := graphql.UpdateAutoScaleConfigMutation(context.Background(), *r.provider.client, plan.Name.Value, rawRegions, true)
		if err != nil {
			resp.Diagnostics.AddError("Update regions failed", err.Error())
		}
	}

	resp.State.Set(ctx, state)

	if resp.Diagnostics.HasError() {
		return
	}
}

func (r flyAppResource) Delete(ctx context.Context, req tfsdk.DeleteResourceRequest, resp *tfsdk.DeleteResourceResponse) {
	var data flyAppResourceData

	diags := req.State.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)

	_, err := graphql.DeleteAppMutation(context.Background(), *r.provider.client, data.Name.Value)
	var errList gqlerror.List
	if errors.As(err, &errList) {
		for _, err := range errList {
			resp.Diagnostics.AddError(err.Message, err.Path.String())
		}
	} else if err != nil {
		resp.Diagnostics.AddError("Delete app failed", err.Error())
	}

	resp.State.RemoveResource(ctx)

	if resp.Diagnostics.HasError() {
		return
	}
}

func (r flyAppResource) ImportState(ctx context.Context, req tfsdk.ImportResourceStateRequest, resp *tfsdk.ImportResourceStateResponse) {
	tfsdk.ResourceImportStatePassthroughID(ctx, tftypes.NewAttributePath().WithAttributeName("id"), req, resp)
}
