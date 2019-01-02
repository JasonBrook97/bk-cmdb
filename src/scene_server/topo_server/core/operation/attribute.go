/*
 * Tencent is pleased to support the open source community by making 蓝鲸 available.
 * Copyright (C) 2017-2018 THL A29 Limited, a Tencent company. All rights reserved.
 * Licensed under the MIT License (the "License"); you may not use this file except
 * in compliance with the License. You may obtain a copy of the License at
 * http://opensource.org/licenses/MIT
 * Unless required by applicable law or agreed to in writing, software distributed under
 * the License is distributed on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND,
 * either express or implied. See the License for the specific language governing permissions and
 * limitations under the License.
 */

package operation

import (
	"context"

	"configcenter/src/apimachinery"
	"configcenter/src/common"
	"configcenter/src/common/blog"
	"configcenter/src/common/condition"
	frtypes "configcenter/src/common/mapstr"
	"configcenter/src/common/metadata"
	"configcenter/src/scene_server/topo_server/core/inst"
	"configcenter/src/scene_server/topo_server/core/model"
	"configcenter/src/scene_server/topo_server/core/types"
)

// AttributeOperationInterface attribute operation methods
type AttributeOperationInterface interface {
	CreateObjectAttribute(params types.ContextParams, data frtypes.MapStr) (model.AttributeInterface, error)
	DeleteObjectAttribute(params types.ContextParams, cond condition.Condition) error
	FindObjectAttributeWithDetail(params types.ContextParams, cond condition.Condition) ([]*metadata.ObjAttDes, error)
	FindObjectAttribute(params types.ContextParams, cond condition.Condition) ([]model.AttributeInterface, error)
	UpdateObjectAttribute(params types.ContextParams, data frtypes.MapStr, attID int64, cond condition.Condition) error

	SetProxy(modelFactory model.Factory, instFactory inst.Factory, obj ObjectOperationInterface, asst AssociationOperationInterface, grp GroupOperationInterface)
}

// NewAttributeOperation create a new attribute operation instance
func NewAttributeOperation(client apimachinery.ClientSetInterface) AttributeOperationInterface {
	return &attribute{
		clientSet: client,
	}
}

type attribute struct {
	clientSet    apimachinery.ClientSetInterface
	modelFactory model.Factory
	instFactory  inst.Factory
	obj          ObjectOperationInterface
	asst         AssociationOperationInterface
	grp          GroupOperationInterface
}

func (a *attribute) SetProxy(modelFactory model.Factory, instFactory inst.Factory, obj ObjectOperationInterface, asst AssociationOperationInterface, grp GroupOperationInterface) {
	a.modelFactory = modelFactory
	a.instFactory = instFactory
	a.obj = obj
	a.asst = asst
	a.grp = grp
}

func (a *attribute) CreateObjectAttribute(params types.ContextParams, data frtypes.MapStr) (model.AttributeInterface, error) {

	att := a.modelFactory.CreateAttribute(params)

	err := att.Parse(data)
	if nil != err {
		blog.Errorf("[operation-attr] failed to parse the attribute data (%#v), error info is %s", data, err.Error())
		return nil, err
	}

	// check the object id
	err = a.obj.IsValidObject(params, att.Attribute().ObjectID)
	if nil != err {
		return nil, params.Err.New(common.CCErrTopoObjectAttributeCreateFailed, err.Error())
	}

	// create a new one
	err = att.Create()
	if nil != err {
		blog.Errorf("[operation-attr] failed to save the attribute data (%#v), error info is %s", data, err.Error())
		return nil, err
	}

	return att, nil
}

func (a *attribute) DeleteObjectAttribute(params types.ContextParams, cond condition.Condition) error {

	attrItems, err := a.FindObjectAttribute(params, cond)
	if nil != err {
		blog.Errorf("[operation-attr] failed to find the attributes by the cond(%v), err: %v", cond.ToMapStr(), err)
		return params.Err.New(common.CCErrTopoObjectAttributeDeleteFailed, err.Error())
	}

	for _, attrItem := range attrItems {
		// delete the attribute
		rsp, err := a.clientSet.ObjectController().Meta().DeleteObjectAttByID(context.Background(), attrItem.Attribute().ID, params.Header, cond.ToMapStr())
		if nil != err {
			blog.Errorf("[operation-attr] delete object attribute failed, request object controller with err: %v", err)
			return params.Err.Error(common.CCErrCommHTTPDoRequestFailed)
		}

		if !rsp.Result {
			blog.Errorf("[operation-attr] failed to delete the attribute by condition(%v), err: %s", cond.ToMapStr(), rsp.ErrMsg)
			return params.Err.New(rsp.Code, rsp.ErrMsg)
		}
	}

	return nil
}
func (a *attribute) FindObjectAttributeWithDetail(params types.ContextParams, cond condition.Condition) ([]*metadata.ObjAttDes, error) {
	attrs, err := a.FindObjectAttribute(params, cond)
	if nil != err {
		return nil, err
	}
	results := make([]*metadata.ObjAttDes, 0)
	for _, attr := range attrs {
		result := &metadata.ObjAttDes{Attribute: *attr.Attribute()}

		attribute := attr.Attribute()
		grpCond := condition.CreateCondition()
		grpCond.Field(metadata.GroupFieldGroupID).Eq(attribute.PropertyGroup)
		grpCond.Field(metadata.GroupFieldSupplierAccount).Eq(attribute.OwnerID)
		grpCond.Field(metadata.GroupFieldObjectID).Eq(attribute.ObjectID)
		grps, err := a.grp.FindObjectGroup(params, grpCond)
		if nil != err {
			return nil, err
		}

		for _, grp := range grps {
			// should be only one
			result.PropertyGroupName = grp.Group().GroupName
		}

		results = append(results, result)
	}

	return results, nil
}
func (a *attribute) FindObjectAttribute(params types.ContextParams, cond condition.Condition) ([]model.AttributeInterface, error) {

	rsp, err := a.clientSet.ObjectController().Meta().SelectObjectAttWithParams(context.Background(), params.Header, cond.ToMapStr())
	if nil != err {
		blog.Errorf("[operation-attr] failed to request object controller, error info is %s", err.Error())
		return nil, params.Err.Error(common.CCErrCommHTTPDoRequestFailed)
	}

	if !rsp.Result {
		blog.Errorf("[operation-attr] failed to search attribute by the condition(%#v), error info is %s", cond.ToMapStr(), rsp.ErrMsg)
		return nil, params.Err.New(rsp.Code, rsp.ErrMsg)
	}

	return model.CreateAttribute(params, a.clientSet, rsp.Data), nil
}

func (a *attribute) UpdateObjectAttribute(params types.ContextParams, data frtypes.MapStr, attID int64, cond condition.Condition) error {

	rsp, err := a.clientSet.ObjectController().Meta().UpdateObjectAttByID(context.Background(), attID, params.Header, data)

	if nil != err {
		blog.Errorf("[operation-attr] failed to request object controller, error info is %s", err.Error())
		return params.Err.Error(common.CCErrCommHTTPDoRequestFailed)
	}

	if !rsp.Result {
		blog.Errorf("[operation-attr] failed to update the attribute by the condition(%#v) or the attr-id(%d), error info is %s", cond.ToMapStr(), attID, rsp.ErrMsg)
		return params.Err.New(rsp.Code, rsp.ErrMsg)
	}

	return nil
}
