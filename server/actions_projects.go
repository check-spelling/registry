// Copyright 2020 Google LLC. All Rights Reserved.

package server

import (
	"context"

	"apigov.dev/registry/models"
	rpc "apigov.dev/registry/rpc"
	"cloud.google.com/go/datastore"
	"github.com/golang/protobuf/ptypes/empty"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func (s *RegistryServer) CreateProject(ctx context.Context, request *rpc.CreateProjectRequest) (*rpc.Project, error) {
	client, err := s.newDataStoreClient(ctx)
	if err != nil {
		return nil, internalError(err)
	}
	defer client.Close()
	project, err := models.NewProjectFromProjectID(request.GetProjectId())
	if err != nil {
		return nil, invalidArgumentError(err)
	}
	k := &datastore.Key{Kind: models.ProjectEntityName, Name: project.ResourceName()}
	// fail if project already exists
	var existingProject models.Project
	err = client.Get(ctx, k, &existingProject)
	if err == nil {
		return nil, status.Error(codes.AlreadyExists, project.ResourceName()+" already exists")
	}
	err = project.Update(request.GetProject())
	project.CreateTime = project.UpdateTime
	k, err = client.Put(ctx, k, project)
	if err != nil {
		return nil, internalError(err)
	}
	return project.Message()
}

func (s *RegistryServer) DeleteProject(ctx context.Context, request *rpc.DeleteProjectRequest) (*empty.Empty, error) {
	client, err := s.newDataStoreClient(ctx)
	if err != nil {
		return nil, internalError(err)
	}
	defer client.Close()
	// Validate name and create dummy project (we just need the ID fields).
	project, err := models.NewProjectFromResourceName(request.GetName())
	if err != nil {
		return nil, invalidArgumentError(err)
	}
	// Delete children first and then delete the project.
	project.DeleteChildren(ctx, client)
	k := &datastore.Key{Kind: models.ProjectEntityName, Name: request.GetName()}
	err = client.Delete(ctx, k)
	return &empty.Empty{}, internalError(err)
}

func (s *RegistryServer) GetProject(ctx context.Context, request *rpc.GetProjectRequest) (*rpc.Project, error) {
	client, err := s.newDataStoreClient(ctx)
	if err != nil {
		return nil, internalError(err)
	}
	defer client.Close()
	project, err := models.NewProjectFromResourceName(request.GetName())
	if err != nil {
		return nil, invalidArgumentError(err)
	}
	k := &datastore.Key{Kind: models.ProjectEntityName, Name: project.ResourceName()}
	err = client.Get(ctx, k, project)
	if err == datastore.ErrNoSuchEntity {
		return nil, status.Error(codes.NotFound, "not found")
	} else if err != nil {
		return nil, internalError(err)
	}
	return project.Message()
}

func (s *RegistryServer) ListProjects(ctx context.Context, req *rpc.ListProjectsRequest) (*rpc.ListProjectsResponse, error) {
	client, err := s.newDataStoreClient(ctx)
	if err != nil {
		return nil, internalError(err)
	}
	defer client.Close()
	q := datastore.NewQuery(models.ProjectEntityName)
	q, err = queryApplyCursor(q, req.GetPageToken())
	if err != nil {
		return nil, internalError(err)
	}
	prg, err := createFilterOperator(req.GetFilter(),
		[]filterArg{
			{"project_id", filterArgTypeString},
			{"availability", filterArgTypeString},
		})
	if err != nil {
		return nil, internalError(err)
	}
	var projectMessages []*rpc.Project
	var project models.Project
	it := client.Run(ctx, q.Distinct())
	pageSize := boundPageSize(req.GetPageSize())
	for _, err = it.Next(&project); err == nil; _, err = it.Next(&project) {
		if prg != nil {
			out, _, err := prg.Eval(map[string]interface{}{
				"project_id": project.ProjectID,
			})
			if err != nil {
				return nil, invalidArgumentError(err)
			}
			if !out.Value().(bool) {
				continue
			}
		}
		projectMessage, _ := project.Message()
		projectMessages = append(projectMessages, projectMessage)
		if len(projectMessages) == pageSize {
			break
		}
	}
	if err != nil && err != iterator.Done {
		return nil, internalError(err)
	}
	responses := &rpc.ListProjectsResponse{
		Projects: projectMessages,
	}
	responses.NextPageToken, err = iteratorGetCursor(it, len(projectMessages))
	if err != nil {
		return nil, internalError(err)
	}
	return responses, nil
}

func (s *RegistryServer) UpdateProject(ctx context.Context, request *rpc.UpdateProjectRequest) (*rpc.Project, error) {
	client, err := s.newDataStoreClient(ctx)
	if err != nil {
		return nil, internalError(err)
	}
	defer client.Close()
	project, err := models.NewProjectFromResourceName(request.GetProject().GetName())
	if err != nil {
		return nil, invalidArgumentError(err)
	}
	k := &datastore.Key{Kind: models.ProjectEntityName, Name: project.ResourceName()}
	err = client.Get(ctx, k, project)
	if err != nil {
		return nil, status.Error(codes.NotFound, "not found")
	}
	err = project.Update(request.GetProject())
	if err != nil {
		return nil, internalError(err)
	}
	k, err = client.Put(ctx, k, project)
	if err != nil {
		return nil, internalError(err)
	}
	return project.Message()
}