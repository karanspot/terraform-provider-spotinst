package spotinst

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/spotinst/spotinst-sdk-go/service/ocean/providers/gcp"
	"github.com/spotinst/spotinst-sdk-go/spotinst"
	"github.com/spotinst/spotinst-sdk-go/spotinst/client"
	"github.com/spotinst/terraform-provider-spotinst/spotinst/commons"
	"github.com/spotinst/terraform-provider-spotinst/spotinst/ocean_gke_import"
	"github.com/spotinst/terraform-provider-spotinst/spotinst/ocean_gke_import_autoscaler"
	"github.com/spotinst/terraform-provider-spotinst/spotinst/ocean_gke_import_launch_specification"
	"github.com/spotinst/terraform-provider-spotinst/spotinst/ocean_gke_import_scheduling"
	"github.com/spotinst/terraform-provider-spotinst/spotinst/ocean_gke_import_strategy"
)

func resourceSpotinstOceanGKEImport() *schema.Resource {
	setupClusterGKEImportResource()

	return &schema.Resource{
		CreateContext: resourceSpotinstClusterGKEImportCreate,
		ReadContext:   resourceSpotinstClusterGKEImportRead,
		UpdateContext: resourceSpotinstClusterGKEImportUpdate,
		DeleteContext: resourceSpotinstClusterGKEImportDelete,

		Importer: &schema.ResourceImporter{
			StateContext: schema.ImportStatePassthroughContext,
		},
		Schema: commons.OceanGKEImportResource.GetSchemaMap(),
	}
}

func setupClusterGKEImportResource() {
	fieldsMap := make(map[commons.FieldName]*commons.GenericField)

	ocean_gke_import.Setup(fieldsMap)
	ocean_gke_import_scheduling.Setup(fieldsMap)
	ocean_gke_import_autoscaler.Setup(fieldsMap)
	ocean_gke_import_launch_specification.Setup(fieldsMap)
	ocean_gke_import_strategy.Setup(fieldsMap)

	commons.OceanGKEImportResource = commons.NewOceanGKEImportResource(fieldsMap)
}

func importOceanGKECluster(resourceData *schema.ResourceData, meta interface{}) (*gcp.Cluster, error) {
	input := &gcp.ImportOceanGKEClusterInput{
		ClusterName: spotinst.String(resourceData.Get("cluster_name").(string)),
		Location:    spotinst.String(resourceData.Get("location").(string)),
	}

	resp, err := meta.(*Client).ocean.CloudProviderGCP().ImportOceanGKECluster(context.Background(), input)

	if err != nil {
		// If the group was not found, return nil so that we can show
		// that the group is gone.
		if errs, ok := err.(client.Errors); ok && len(errs) > 0 {
			for _, err := range errs {
				if err.Code == ErrCodeClusterNotFound {
					resourceData.SetId("")
					return nil, err
				}
			}
		}
		// Some other error, report it.
		return nil, fmt.Errorf("ocean GKE: import failed to read group: %s", err)
	}

	return resp.Cluster, err
}

func resourceSpotinstClusterGKEImportCreate(ctx context.Context, resourceData *schema.ResourceData, meta interface{}) diag.Diagnostics {
	log.Printf(string(commons.ResourceOnCreate),
		commons.OceanGKEImportResource.GetName())

	importedCluster, err := importOceanGKECluster(resourceData, meta.(*Client))
	if err != nil {
		return diag.FromErr(err)
	}

	cluster, err := commons.OceanGKEImportResource.OnCreate(importedCluster, resourceData, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	clusterID, err := createGKEImportedCluster(cluster, meta.(*Client))
	if err != nil {
		return diag.FromErr(err)
	}

	resourceData.SetId(spotinst.StringValue(clusterID))

	log.Printf("===> GKE imported cluster created successfully: %s <===", resourceData.Id())
	return resourceSpotinstClusterGKEImportRead(ctx, resourceData, meta)
}

func createGKEImportedCluster(cluster *gcp.Cluster, spotinstClient *Client) (*string, error) {
	if json, err := commons.ToJson(cluster); err != nil {
		return nil, err
	} else {
		log.Printf("===> Cluster GKE import configuration: %s", json)
	}

	var resp *gcp.CreateClusterOutput = nil
	err := resource.RetryContext(context.Background(), time.Minute, func() *resource.RetryError {
		input := &gcp.CreateClusterInput{Cluster: cluster}
		r, err := spotinstClient.ocean.CloudProviderGCP().CreateCluster(context.Background(), input)
		if err != nil {

			// Some other error, report it.
			return resource.NonRetryableError(err)
		}
		resp = r
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("[ERROR] failed to create GKE imported cluster: %s", err)
	}
	return resp.Cluster.ID, nil
}

func resourceSpotinstClusterGKEImportRead(ctx context.Context, resourceData *schema.ResourceData, meta interface{}) diag.Diagnostics {
	id := resourceData.Id()
	log.Printf(string(commons.ResourceOnRead),
		commons.OceanGKEImportResource.GetName(), id)

	input := &gcp.ReadClusterInput{ClusterID: spotinst.String(id)}
	resp, err := meta.(*Client).ocean.CloudProviderGCP().ReadCluster(context.Background(), input)

	if err != nil {
		// If the cluster was not found, return nil so that we can show
		// that the group does not exist
		if errs, ok := err.(client.Errors); ok && len(errs) > 0 {
			for _, err := range errs {
				if err.Code == ErrCodeClusterNotFound {
					resourceData.SetId("")
					return nil
				}
			}
		}

		// Some other error, report it.
		return diag.Errorf("failed to read GKE cluster: %s", err)
	}

	// if nothing was found, return no state
	clusterResponse := resp.Cluster
	if clusterResponse == nil {
		resourceData.SetId("")
		return nil
	}

	// Expose the controller cluster identifier.
	if clusterResponse.ControllerClusterID != nil {
		for _, key := range []commons.FieldName{
			ocean_gke_import.ControllerClusterID,
			ocean_gke_import.ClusterControllerID, // maintains backward compatibility
		} {
			if err := resourceData.Set(string(key), *clusterResponse.ControllerClusterID); err != nil {
				log.Printf("[ERROR] Failed to set %q", string(key))
			}
		}
	}

	if err := commons.OceanGKEImportResource.OnRead(clusterResponse, resourceData, meta); err != nil {
		return diag.FromErr(err)
	}
	log.Printf("===> GKE cluster read successfully: %s <===", id)
	return nil
}

func resourceSpotinstClusterGKEImportUpdate(ctx context.Context, resourceData *schema.ResourceData, meta interface{}) diag.Diagnostics {
	id := resourceData.Id()
	log.Printf(string(commons.ResourceOnUpdate),
		commons.OceanGKEImportResource.GetName(), id)

	shouldUpdate, changesRequiredRoll, cluster, err := commons.OceanGKEImportResource.OnUpdate(resourceData, meta)
	if err != nil {
		return diag.FromErr(err)
	}

	if shouldUpdate {
		cluster.SetId(spotinst.String(id))
		if err := updateGKEImportCluster(cluster, resourceData, meta, changesRequiredRoll); err != nil {
			return diag.FromErr(err)
		}
	}
	log.Printf("===> GLE Cluster updated successfully: %s <===", id)
	return resourceSpotinstClusterGKEImportRead(ctx, resourceData, meta)
}

func updateGKEImportCluster(cluster *gcp.Cluster, resourceData *schema.ResourceData, meta interface{}, changesRequiredRoll bool) error {
	var input = &gcp.UpdateClusterInput{
		Cluster: cluster,
	}

	var shouldRoll = false
	var conditionedRoll = false
	clusterID := resourceData.Id()
	if updatePolicy, exists := resourceData.GetOkExists(string(ocean_gke_import.UpdatePolicy)); exists {
		list := updatePolicy.([]interface{})
		if len(list) > 0 && list[0] != nil {
			m := list[0].(map[string]interface{})

			if roll, ok := m[string(ocean_gke_import.ShouldRoll)].(bool); ok && roll {
				shouldRoll = roll
			}

			if condRoll, ok := m[string(ocean_gke_import.ConditionedRoll)].(bool); ok && condRoll {
				conditionedRoll = condRoll
			}
		}
	}

	if json, err := commons.ToJson(cluster); err != nil {
		return err
	} else {
		log.Printf("===> GKE Cluster update configuration: %s", json)
	}

	if _, err := meta.(*Client).ocean.CloudProviderGCP().UpdateCluster(context.Background(), input); err != nil {
		return fmt.Errorf("[ERROR] Failed to update GKE cluster [%v]: %v", clusterID, err)
	} else if shouldRoll {
		if !conditionedRoll || changesRequiredRoll {
			if err := rollOceanGKECluster(resourceData, meta); err != nil {
				log.Printf("[ERROR] Cluster [%v] roll failed, error: %v", clusterID, err)
				return err
			}
		}
	} else {
		log.Printf("onRoll() -> Field [%v] is false, skipping cluster roll", string(ocean_gke_import.ShouldRoll))
	}

	return nil
}

func resourceSpotinstClusterGKEImportDelete(ctx context.Context, resourceData *schema.ResourceData, meta interface{}) diag.Diagnostics {
	id := resourceData.Id()
	log.Printf(string(commons.ResourceOnDelete),
		commons.OceanGKEImportResource.GetName(), id)

	if err := deleteGKEImportCluster(resourceData, meta); err != nil {
		return diag.FromErr(err)
	}

	log.Printf("===> GKE Cluster deleted successfully: %s <===", resourceData.Id())
	resourceData.SetId("")
	return nil
}

func deleteGKEImportCluster(resourceData *schema.ResourceData, meta interface{}) error {
	clusterID := resourceData.Id()
	input := &gcp.DeleteClusterInput{
		ClusterID: spotinst.String(clusterID),
	}

	if json, err := commons.ToJson(input); err != nil {
		return err
	} else {
		log.Printf("===> GKE Cluster delete configuration: %s", json)
	}

	if _, err := meta.(*Client).ocean.CloudProviderGCP().DeleteCluster(context.Background(), input); err != nil {
		return fmt.Errorf("[ERROR] onDelete() -> Failed to delete GKE cluster: %s", err)
	}
	return nil
}

func rollOceanGKECluster(resourceData *schema.ResourceData, meta interface{}) error {
	clusterID := resourceData.Id()

	updatePolicy, exists := resourceData.GetOkExists(string(ocean_gke_import.UpdatePolicy))
	if !exists {
		return fmt.Errorf("ocean/gke: missing update policy for cluster %q", clusterID)
	}

	list := updatePolicy.([]interface{})
	if len(list) > 0 && list[0] != nil {
		updateClusterSchema := list[0].(map[string]interface{})

		rollConfig, ok := updateClusterSchema[string(ocean_gke_import.RollConfig)]
		if !ok || rollConfig == nil {
			return fmt.Errorf("ocean/gke: missing roll configuration, "+
				"skipping roll for cluster %q", clusterID)
		}

		rollSpec, err := expandOceanGKEClusterRollConfig(rollConfig, clusterID)
		if err != nil {
			return fmt.Errorf("ocean/gke: failed expanding roll "+
				"configuration for cluster %q, error: %v", clusterID, err)
		}

		rollJSON, err := commons.ToJson(rollConfig)
		if err != nil {
			return fmt.Errorf("ocean/gke: failed marshaling roll "+
				"configuration for cluster %q, error: %v", clusterID, err)
		}

		log.Printf("onRoll() -> Rolling cluster [%v] with configuration %s", clusterID, rollJSON)
		rollInput := &gcp.CreateRollInput{Roll: rollSpec}
		if _, err = meta.(*Client).ocean.CloudProviderGCP().CreateRoll(context.TODO(), rollInput); err != nil {
			return fmt.Errorf("onRoll() -> Roll failed for cluster [%v], error: %v", clusterID, err)
		}
		log.Printf("onRoll() -> Successfully rolled cluster [%v]", clusterID)
	}

	return nil
}

func expandOceanGKEClusterRollConfig(data interface{}, clusterID string) (*gcp.RollSpec, error) {
	list := data.([]interface{})
	spec := &gcp.RollSpec{
		ClusterID: spotinst.String(clusterID),
	}

	if list != nil && list[0] != nil {
		m := list[0].(map[string]interface{})

		if v, ok := m[string(ocean_gke_import.BatchSizePercentage)].(int); ok {
			spec.BatchSizePercentage = spotinst.Int(v)
		}

		if v, ok := m[string(ocean_gke_import.LaunchSpecIDs)].([]string); ok {
			spec.LaunchSpecIDs = expandOceanGKELaunchSpecIDs(v)
		}

		if v, ok := m[string(ocean_gke_import.BatchMinHealthyPercentage)].(int); ok && v > 0 {
			spec.BatchMinHealthyPercentage = spotinst.Int(v)
		}
	}

	return spec, nil
}

func expandOceanGKELaunchSpecIDs(data interface{}) []string {
	list := data.([]interface{})
	result := make([]string, 0, len(list))

	for _, v := range list {
		if ls, ok := v.(string); ok && ls != "" {
			result = append(result, ls)
		}
	}

	return result
}
