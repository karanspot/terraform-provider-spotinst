package spotinst

import (
	"context"
	"fmt"
	"github.com/spotinst/terraform-provider-spotinst/spotinst/azure_v3/stateful_node_azure"
	"log"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/helper/schema"
	v3 "github.com/spotinst/spotinst-sdk-go/service/stateful/providers/azure"
	"github.com/spotinst/spotinst-sdk-go/spotinst"
	"github.com/spotinst/spotinst-sdk-go/spotinst/client"
	"github.com/spotinst/terraform-provider-spotinst/spotinst/commons"
)

func resourceSpotinstStatefulNodeAzureV3() *schema.Resource {
	setupStatefulNodeAzureV3Resource()

	return &schema.Resource{
		Create: resourceSpotinstStatefulNodeAzureV3Create,
		Read:   resourceSpotinstStatefulNodeAzureV3Read,
		Update: resourceSpotinstStatefulNodeAzureV3Update,
		Delete: resourceSpotinstStatefulNodeAzureV3Delete,

		//TODO - need to add all additional methods as part of create/update (see example in Ocean AWS - roll)

		Importer: &schema.ResourceImporter{
			State: schema.ImportStatePassthrough,
		},

		Schema: commons.StatefulNodeAzureV3Resource.GetSchemaMap(),
	}
}

func setupStatefulNodeAzureV3Resource() {
	fieldsMap := make(map[commons.FieldName]*commons.GenericField)

	stateful_node_azure.Setup(fieldsMap)

	//TODO - add all of the rest here

	//elastigroup_azure_image.Setup(fieldsMap)
	//elastigroup_azure_login.Setup(fieldsMap)
	//elastigroup_azure_network.Setup(fieldsMap)
	//elastigroup_azure_strategy.Setup(fieldsMap)
	//elastigroup_azure_vm_sizes.Setup(fieldsMap)
	//elastigroup_azure_launchspecification.Setup(fieldsMap)

	commons.StatefulNodeAzureV3Resource = commons.NewStatefulNodeAzureV3Resource(fieldsMap)
}

func resourceSpotinstStatefulNodeAzureV3Create(resourceData *schema.ResourceData, meta interface{}) error {
	log.Printf(string(commons.ResourceOnCreate),
		commons.StatefulNodeAzureV3Resource.GetName())

	statefulNode, err := commons.StatefulNodeAzureV3Resource.OnCreate(resourceData, meta)
	if err != nil {
		return err
	}

	statefulNodeId, err := createAzureV3StatefulNode(statefulNode, meta.(*Client))
	if err != nil {
		return err
	}

	resourceData.SetId(spotinst.StringValue(statefulNodeId))

	log.Printf("===> Stateful node created successfully: %s <===", resourceData.Id())

	return resourceSpotinstStatefulNodeAzureV3Read(resourceData, meta)
}

func createAzureV3StatefulNode(statefulNode *v3.StatefulNode, spotinstClient *Client) (*string, error) {
	if json, err := commons.ToJson(statefulNode); err != nil {
		return nil, err
	} else {
		log.Printf("===> Stateful node create configuration: %s", json)
	}

	var resp *v3.CreateStatefulNodeOutput = nil
	err := resource.Retry(time.Minute, func() *resource.RetryError {
		input := &v3.CreateStatefulNodeInput{StatefulNode: statefulNode}
		r, err := spotinstClient.statefulNode.CloudProviderAzure().Create(context.Background(), input)
		if err != nil {
			log.Printf("error: %v", err)
			// Some other error, report it.
			return resource.NonRetryableError(err)
		}
		resp = r
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("[ERROR] failed to create stateful node: %s", err)
	}
	return resp.StatefulNode.ID, nil
}

func resourceSpotinstStatefulNodeAzureV3Read(resourceData *schema.ResourceData, meta interface{}) error {
	id := resourceData.Id()
	log.Printf(string(commons.ResourceFieldOnRead),
		commons.StatefulNodeAzureV3Resource.GetName(), id)

	input := &v3.ReadStatefulNodeInput{ID: spotinst.String(id)}
	resp, err := meta.(*Client).statefulNode.CloudProviderAzure().Read(context.Background(), input)
	if err != nil {
		// If the stateful node was not found, return nil so that we can show
		// that the stateful node does not exist
		if errs, ok := err.(client.Errors); ok && len(errs) > 0 {
			for _, err := range errs {
				if err.Code == ErrCodeGroupNotFound {
					resourceData.SetId("")
					return nil
				}
			}
		}

		// Some other error, report it.
		return fmt.Errorf("failed to read stateful node: %s", err)
	}

	// If nothing was found, then return no state.
	statefulNodeResponse := resp.StatefulNode
	if statefulNodeResponse == nil {
		resourceData.SetId("")
		return nil
	}

	if err := commons.StatefulNodeAzureV3Resource.OnRead(statefulNodeResponse, resourceData, meta); err != nil {
		return err
	}
	log.Printf("===> Stateful node read successfully: %s <===", id)
	return nil
}

func resourceSpotinstStatefulNodeAzureV3Update(resourceData *schema.ResourceData, meta interface{}) error {
	id := resourceData.Id()
	log.Printf(string(commons.ResourceOnUpdate),
		commons.StatefulNodeAzureV3Resource.GetName(), id)

	shouldUpdate, statefulNode, err := commons.StatefulNodeAzureV3Resource.OnUpdate(resourceData, meta)
	if err != nil {
		return err
	}

	if shouldUpdate {
		statefulNode.SetID(spotinst.String(id))
		if err := updateAzureV3StatefulNode(statefulNode, resourceData, meta); err != nil {
			return err
		}
	}

	log.Printf("===> Stateful node updated successfully: %s <===", id)
	return resourceSpotinstStatefulNodeAzureV3Read(resourceData, meta)
}

func updateAzureV3StatefulNode(statefulNode *v3.StatefulNode, resourceData *schema.ResourceData, meta interface{}) error {
	var input = &v3.UpdateStatefulNodeInput{
		StatefulNode: statefulNode,
	}

	statefulNodeId := resourceData.Id()
	var shouldUpdateState = false

	//TODO - attach & detach
	//var shouldDetachDataDisk = false
	//var shouldAttachDataDisk = false
	if updateState, ok := resourceData.GetOk(string(stateful_node_azure.UpdateState)); ok {
		list := updateState.([]interface{})
		if len(list) > 0 && list[0] != nil {
			m := list[0].(map[string]interface{})
			if state, ok := m[string(stateful_node_azure.State)].(string); ok && state != "" {
				shouldUpdateState = true
			}

		}
	}

	if json, err := commons.ToJson(statefulNode); err != nil {
		return err
	} else {
		log.Printf("===> Stateful node update configuration: %s", json)
	}

	if _, err := meta.(*Client).statefulNode.CloudProviderAzure().Update(context.Background(), input); err != nil {
		return fmt.Errorf("[ERROR] Failed to update stateful node [%v]: %v", statefulNodeId, err)
	} else if shouldUpdateState {
		if err := updateStateAzureV3StatefulNode(resourceData, meta); err != nil {
			log.Printf("[ERROR] Stateful node [%v] state update failed, error: %v", statefulNodeId, err)
			return err
		}
	} else {
		log.Printf("onUpdate() -> Field [%v] is missing, skipping state update for stateful node",
			string(stateful_node_azure.UpdateState))
	}
	return nil
}

func updateStateAzureV3StatefulNode(resourceData *schema.ResourceData, meta interface{}) error {
	statefulNodeID := resourceData.Id()

	updateState, ok := resourceData.GetOk(string(stateful_node_azure.UpdateState))
	if !ok {
		return fmt.Errorf("stateful node/azure: missing update state for stateful node %q", statefulNodeID)
	}

	list := updateState.([]interface{})
	if len(list) > 0 && list[0] != nil {
		updateStatefulNodeStateSchema := list[0].(map[string]interface{})

		state, ok := updateStatefulNodeStateSchema[string(stateful_node_azure.State)]
		if !ok || state == nil {
			return fmt.Errorf("ocean/aws: missing new state configuration, "+
				"skipping roll for cluster %q", statefulNodeID)
		}

		updateStateSpec, err := expandUpdateStateStatefulNodeAzure(updateState, statefulNodeID)
		if err != nil {
			return fmt.Errorf("stateful node/azure: failed expanding state update "+
				"configuration for stateful node %q, error: %v", statefulNodeID, err)
		}

		updateStateJSON, err := commons.ToJson(state)
		if err != nil {
			return fmt.Errorf("stateful node/azure: failed marshaling state update "+
				"configuration for stateful node %q, error: %v", statefulNodeID, err)
		}

		log.Printf("onUpdate() -> Updating stateful node [%v] with configuration %s", statefulNodeID, updateStateJSON)
		updateStateInput := &v3.UpdateStatefulNodeStateInput{ID: updateStateSpec.ID,
			StatefulNodeState: updateStateSpec.StatefulNodeState}
		if _, err = meta.(*Client).statefulNode.CloudProviderAzure().UpdateState(context.TODO(),
			updateStateInput); err != nil {
			return fmt.Errorf("onUpdate() -> State update failed for stateful node [%v], error: %v",
				statefulNodeID, err)
		}
		log.Printf("onUpdate() -> Successfully updated state for stateful node [%v]", statefulNodeID)
	}

	return nil
}

func expandUpdateStateStatefulNodeAzure(data interface{}, statefulNodeID string) (*v3.UpdateStatefulNodeStateInput, error) {
	list := data.([]interface{})
	spec := &v3.UpdateStatefulNodeStateInput{
		ID: spotinst.String(statefulNodeID),
	}

	if list != nil && list[0] != nil {
		m := list[0].(map[string]interface{})

		if v, ok := m[string(stateful_node_azure.State)].(string); ok && v != "" {
			spec.StatefulNodeState = spotinst.String(v)
		}
	}

	return spec, nil
}

func resourceSpotinstStatefulNodeAzureV3Delete(resourceData *schema.ResourceData, meta interface{}) error {
	id := resourceData.Id()
	log.Printf(string(commons.ResourceOnDelete),
		commons.StatefulNodeAzureV3Resource.GetName(), id)

	if err := deleteAzureV3StatefulNode(resourceData, meta); err != nil {
		return err
	}

	log.Printf("===> Stateful node deleted successfully: %s <===", resourceData.Id())
	resourceData.SetId("")
	return nil
}

func deleteAzureV3StatefulNode(resourceData *schema.ResourceData, meta interface{}) error {
	statefulNodeId := resourceData.Id()
	input := &v3.DeleteStatefulNodeInput{
		ID: spotinst.String(statefulNodeId),
	}

	if json, err := commons.ToJson(input); err != nil {
		return err
	} else {
		log.Printf("===> Stateful node delete configuration: %s", json)
	}

	if _, err := meta.(*Client).statefulNode.CloudProviderAzure().Delete(context.Background(), input); err != nil {
		return fmt.Errorf("[ERROR] onDelete() -> Failed to delete stateful node: %s", err)
	}
	return nil
}