// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - protoc             v3.21.9
// source: jamsyncpb.proto

package jamsyncpb

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// JamsyncAPIClient is the client API for JamsyncAPI service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type JamsyncAPIClient interface {
	AddUser(ctx context.Context, in *AddUserRequest, opts ...grpc.CallOption) (*AddUserResponse, error)
	GetUser(ctx context.Context, in *GetUserRequest, opts ...grpc.CallOption) (*GetUserResponse, error)
	ListUsers(ctx context.Context, in *ListUsersRequest, opts ...grpc.CallOption) (*ListUsersResponse, error)
	AddProject(ctx context.Context, in *AddProjectRequest, opts ...grpc.CallOption) (*AddProjectResponse, error)
	GetProject(ctx context.Context, in *GetProjectRequest, opts ...grpc.CallOption) (*GetProjectResponse, error)
	ListProjects(ctx context.Context, in *ListProjectsRequest, opts ...grpc.CallOption) (*ListProjectsResponse, error)
	UpdateStream(ctx context.Context, opts ...grpc.CallOption) (JamsyncAPI_UpdateStreamClient, error)
}

type jamsyncAPIClient struct {
	cc grpc.ClientConnInterface
}

func NewJamsyncAPIClient(cc grpc.ClientConnInterface) JamsyncAPIClient {
	return &jamsyncAPIClient{cc}
}

func (c *jamsyncAPIClient) AddUser(ctx context.Context, in *AddUserRequest, opts ...grpc.CallOption) (*AddUserResponse, error) {
	out := new(AddUserResponse)
	err := c.cc.Invoke(ctx, "/jamsyncpb.JamsyncAPI/AddUser", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *jamsyncAPIClient) GetUser(ctx context.Context, in *GetUserRequest, opts ...grpc.CallOption) (*GetUserResponse, error) {
	out := new(GetUserResponse)
	err := c.cc.Invoke(ctx, "/jamsyncpb.JamsyncAPI/GetUser", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *jamsyncAPIClient) ListUsers(ctx context.Context, in *ListUsersRequest, opts ...grpc.CallOption) (*ListUsersResponse, error) {
	out := new(ListUsersResponse)
	err := c.cc.Invoke(ctx, "/jamsyncpb.JamsyncAPI/ListUsers", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *jamsyncAPIClient) AddProject(ctx context.Context, in *AddProjectRequest, opts ...grpc.CallOption) (*AddProjectResponse, error) {
	out := new(AddProjectResponse)
	err := c.cc.Invoke(ctx, "/jamsyncpb.JamsyncAPI/AddProject", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *jamsyncAPIClient) GetProject(ctx context.Context, in *GetProjectRequest, opts ...grpc.CallOption) (*GetProjectResponse, error) {
	out := new(GetProjectResponse)
	err := c.cc.Invoke(ctx, "/jamsyncpb.JamsyncAPI/GetProject", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *jamsyncAPIClient) ListProjects(ctx context.Context, in *ListProjectsRequest, opts ...grpc.CallOption) (*ListProjectsResponse, error) {
	out := new(ListProjectsResponse)
	err := c.cc.Invoke(ctx, "/jamsyncpb.JamsyncAPI/ListProjects", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *jamsyncAPIClient) UpdateStream(ctx context.Context, opts ...grpc.CallOption) (JamsyncAPI_UpdateStreamClient, error) {
	stream, err := c.cc.NewStream(ctx, &JamsyncAPI_ServiceDesc.Streams[0], "/jamsyncpb.JamsyncAPI/UpdateStream", opts...)
	if err != nil {
		return nil, err
	}
	x := &jamsyncAPIUpdateStreamClient{stream}
	return x, nil
}

type JamsyncAPI_UpdateStreamClient interface {
	Send(*UpdateStreamRequest) error
	Recv() (*UpdateStreamResponse, error)
	grpc.ClientStream
}

type jamsyncAPIUpdateStreamClient struct {
	grpc.ClientStream
}

func (x *jamsyncAPIUpdateStreamClient) Send(m *UpdateStreamRequest) error {
	return x.ClientStream.SendMsg(m)
}

func (x *jamsyncAPIUpdateStreamClient) Recv() (*UpdateStreamResponse, error) {
	m := new(UpdateStreamResponse)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// JamsyncAPIServer is the server API for JamsyncAPI service.
// All implementations must embed UnimplementedJamsyncAPIServer
// for forward compatibility
type JamsyncAPIServer interface {
	AddUser(context.Context, *AddUserRequest) (*AddUserResponse, error)
	GetUser(context.Context, *GetUserRequest) (*GetUserResponse, error)
	ListUsers(context.Context, *ListUsersRequest) (*ListUsersResponse, error)
	AddProject(context.Context, *AddProjectRequest) (*AddProjectResponse, error)
	GetProject(context.Context, *GetProjectRequest) (*GetProjectResponse, error)
	ListProjects(context.Context, *ListProjectsRequest) (*ListProjectsResponse, error)
	UpdateStream(JamsyncAPI_UpdateStreamServer) error
	mustEmbedUnimplementedJamsyncAPIServer()
}

// UnimplementedJamsyncAPIServer must be embedded to have forward compatible implementations.
type UnimplementedJamsyncAPIServer struct {
}

func (UnimplementedJamsyncAPIServer) AddUser(context.Context, *AddUserRequest) (*AddUserResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method AddUser not implemented")
}
func (UnimplementedJamsyncAPIServer) GetUser(context.Context, *GetUserRequest) (*GetUserResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetUser not implemented")
}
func (UnimplementedJamsyncAPIServer) ListUsers(context.Context, *ListUsersRequest) (*ListUsersResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListUsers not implemented")
}
func (UnimplementedJamsyncAPIServer) AddProject(context.Context, *AddProjectRequest) (*AddProjectResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method AddProject not implemented")
}
func (UnimplementedJamsyncAPIServer) GetProject(context.Context, *GetProjectRequest) (*GetProjectResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetProject not implemented")
}
func (UnimplementedJamsyncAPIServer) ListProjects(context.Context, *ListProjectsRequest) (*ListProjectsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListProjects not implemented")
}
func (UnimplementedJamsyncAPIServer) UpdateStream(JamsyncAPI_UpdateStreamServer) error {
	return status.Errorf(codes.Unimplemented, "method UpdateStream not implemented")
}
func (UnimplementedJamsyncAPIServer) mustEmbedUnimplementedJamsyncAPIServer() {}

// UnsafeJamsyncAPIServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to JamsyncAPIServer will
// result in compilation errors.
type UnsafeJamsyncAPIServer interface {
	mustEmbedUnimplementedJamsyncAPIServer()
}

func RegisterJamsyncAPIServer(s grpc.ServiceRegistrar, srv JamsyncAPIServer) {
	s.RegisterService(&JamsyncAPI_ServiceDesc, srv)
}

func _JamsyncAPI_AddUser_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(AddUserRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(JamsyncAPIServer).AddUser(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/jamsyncpb.JamsyncAPI/AddUser",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(JamsyncAPIServer).AddUser(ctx, req.(*AddUserRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _JamsyncAPI_GetUser_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetUserRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(JamsyncAPIServer).GetUser(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/jamsyncpb.JamsyncAPI/GetUser",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(JamsyncAPIServer).GetUser(ctx, req.(*GetUserRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _JamsyncAPI_ListUsers_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListUsersRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(JamsyncAPIServer).ListUsers(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/jamsyncpb.JamsyncAPI/ListUsers",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(JamsyncAPIServer).ListUsers(ctx, req.(*ListUsersRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _JamsyncAPI_AddProject_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(AddProjectRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(JamsyncAPIServer).AddProject(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/jamsyncpb.JamsyncAPI/AddProject",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(JamsyncAPIServer).AddProject(ctx, req.(*AddProjectRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _JamsyncAPI_GetProject_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetProjectRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(JamsyncAPIServer).GetProject(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/jamsyncpb.JamsyncAPI/GetProject",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(JamsyncAPIServer).GetProject(ctx, req.(*GetProjectRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _JamsyncAPI_ListProjects_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListProjectsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(JamsyncAPIServer).ListProjects(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/jamsyncpb.JamsyncAPI/ListProjects",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(JamsyncAPIServer).ListProjects(ctx, req.(*ListProjectsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _JamsyncAPI_UpdateStream_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(JamsyncAPIServer).UpdateStream(&jamsyncAPIUpdateStreamServer{stream})
}

type JamsyncAPI_UpdateStreamServer interface {
	Send(*UpdateStreamResponse) error
	Recv() (*UpdateStreamRequest, error)
	grpc.ServerStream
}

type jamsyncAPIUpdateStreamServer struct {
	grpc.ServerStream
}

func (x *jamsyncAPIUpdateStreamServer) Send(m *UpdateStreamResponse) error {
	return x.ServerStream.SendMsg(m)
}

func (x *jamsyncAPIUpdateStreamServer) Recv() (*UpdateStreamRequest, error) {
	m := new(UpdateStreamRequest)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// JamsyncAPI_ServiceDesc is the grpc.ServiceDesc for JamsyncAPI service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var JamsyncAPI_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "jamsyncpb.JamsyncAPI",
	HandlerType: (*JamsyncAPIServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "AddUser",
			Handler:    _JamsyncAPI_AddUser_Handler,
		},
		{
			MethodName: "GetUser",
			Handler:    _JamsyncAPI_GetUser_Handler,
		},
		{
			MethodName: "ListUsers",
			Handler:    _JamsyncAPI_ListUsers_Handler,
		},
		{
			MethodName: "AddProject",
			Handler:    _JamsyncAPI_AddProject_Handler,
		},
		{
			MethodName: "GetProject",
			Handler:    _JamsyncAPI_GetProject_Handler,
		},
		{
			MethodName: "ListProjects",
			Handler:    _JamsyncAPI_ListProjects_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "UpdateStream",
			Handler:       _JamsyncAPI_UpdateStream_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
	},
	Metadata: "jamsyncpb.proto",
}
