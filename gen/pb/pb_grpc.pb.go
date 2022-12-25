// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.2.0
// - protoc             v3.21.9
// source: pb.proto

package pb

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
	CreateChange(ctx context.Context, in *CreateChangeRequest, opts ...grpc.CallOption) (*CreateChangeResponse, error)
	WriteOperationStream(ctx context.Context, opts ...grpc.CallOption) (JamsyncAPI_WriteOperationStreamClient, error)
	CommitChange(ctx context.Context, in *CommitChangeRequest, opts ...grpc.CallOption) (*CommitChangeResponse, error)
	ReadOperationStream(ctx context.Context, opts ...grpc.CallOption) (JamsyncAPI_ReadOperationStreamClient, error)
	ReadBlockHashes(ctx context.Context, in *ReadBlockHashesRequest, opts ...grpc.CallOption) (*ReadBlockHashesResponse, error)
	ReadFile(ctx context.Context, in *ReadFileRequest, opts ...grpc.CallOption) (JamsyncAPI_ReadFileClient, error)
	AddProject(ctx context.Context, in *AddProjectRequest, opts ...grpc.CallOption) (*AddProjectResponse, error)
	ListProjects(ctx context.Context, in *ListProjectsRequest, opts ...grpc.CallOption) (*ListProjectsResponse, error)
	BrowseProject(ctx context.Context, in *BrowseProjectRequest, opts ...grpc.CallOption) (*BrowseProjectResponse, error)
	UserInfo(ctx context.Context, in *UserInfoRequest, opts ...grpc.CallOption) (*UserInfoResponse, error)
	CreateUser(ctx context.Context, in *CreateUserRequest, opts ...grpc.CallOption) (*CreateUserResponse, error)
}

type jamsyncAPIClient struct {
	cc grpc.ClientConnInterface
}

func NewJamsyncAPIClient(cc grpc.ClientConnInterface) JamsyncAPIClient {
	return &jamsyncAPIClient{cc}
}

func (c *jamsyncAPIClient) CreateChange(ctx context.Context, in *CreateChangeRequest, opts ...grpc.CallOption) (*CreateChangeResponse, error) {
	out := new(CreateChangeResponse)
	err := c.cc.Invoke(ctx, "/pb.JamsyncAPI/CreateChange", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *jamsyncAPIClient) WriteOperationStream(ctx context.Context, opts ...grpc.CallOption) (JamsyncAPI_WriteOperationStreamClient, error) {
	stream, err := c.cc.NewStream(ctx, &JamsyncAPI_ServiceDesc.Streams[0], "/pb.JamsyncAPI/WriteOperationStream", opts...)
	if err != nil {
		return nil, err
	}
	x := &jamsyncAPIWriteOperationStreamClient{stream}
	return x, nil
}

type JamsyncAPI_WriteOperationStreamClient interface {
	Send(*Operation) error
	Recv() (*OperationLocation, error)
	grpc.ClientStream
}

type jamsyncAPIWriteOperationStreamClient struct {
	grpc.ClientStream
}

func (x *jamsyncAPIWriteOperationStreamClient) Send(m *Operation) error {
	return x.ClientStream.SendMsg(m)
}

func (x *jamsyncAPIWriteOperationStreamClient) Recv() (*OperationLocation, error) {
	m := new(OperationLocation)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *jamsyncAPIClient) CommitChange(ctx context.Context, in *CommitChangeRequest, opts ...grpc.CallOption) (*CommitChangeResponse, error) {
	out := new(CommitChangeResponse)
	err := c.cc.Invoke(ctx, "/pb.JamsyncAPI/CommitChange", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *jamsyncAPIClient) ReadOperationStream(ctx context.Context, opts ...grpc.CallOption) (JamsyncAPI_ReadOperationStreamClient, error) {
	stream, err := c.cc.NewStream(ctx, &JamsyncAPI_ServiceDesc.Streams[1], "/pb.JamsyncAPI/ReadOperationStream", opts...)
	if err != nil {
		return nil, err
	}
	x := &jamsyncAPIReadOperationStreamClient{stream}
	return x, nil
}

type JamsyncAPI_ReadOperationStreamClient interface {
	Send(*OperationLocation) error
	Recv() (*Operation, error)
	grpc.ClientStream
}

type jamsyncAPIReadOperationStreamClient struct {
	grpc.ClientStream
}

func (x *jamsyncAPIReadOperationStreamClient) Send(m *OperationLocation) error {
	return x.ClientStream.SendMsg(m)
}

func (x *jamsyncAPIReadOperationStreamClient) Recv() (*Operation, error) {
	m := new(Operation)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *jamsyncAPIClient) ReadBlockHashes(ctx context.Context, in *ReadBlockHashesRequest, opts ...grpc.CallOption) (*ReadBlockHashesResponse, error) {
	out := new(ReadBlockHashesResponse)
	err := c.cc.Invoke(ctx, "/pb.JamsyncAPI/ReadBlockHashes", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *jamsyncAPIClient) ReadFile(ctx context.Context, in *ReadFileRequest, opts ...grpc.CallOption) (JamsyncAPI_ReadFileClient, error) {
	stream, err := c.cc.NewStream(ctx, &JamsyncAPI_ServiceDesc.Streams[2], "/pb.JamsyncAPI/ReadFile", opts...)
	if err != nil {
		return nil, err
	}
	x := &jamsyncAPIReadFileClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type JamsyncAPI_ReadFileClient interface {
	Recv() (*Operation, error)
	grpc.ClientStream
}

type jamsyncAPIReadFileClient struct {
	grpc.ClientStream
}

func (x *jamsyncAPIReadFileClient) Recv() (*Operation, error) {
	m := new(Operation)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func (c *jamsyncAPIClient) AddProject(ctx context.Context, in *AddProjectRequest, opts ...grpc.CallOption) (*AddProjectResponse, error) {
	out := new(AddProjectResponse)
	err := c.cc.Invoke(ctx, "/pb.JamsyncAPI/AddProject", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *jamsyncAPIClient) ListProjects(ctx context.Context, in *ListProjectsRequest, opts ...grpc.CallOption) (*ListProjectsResponse, error) {
	out := new(ListProjectsResponse)
	err := c.cc.Invoke(ctx, "/pb.JamsyncAPI/ListProjects", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *jamsyncAPIClient) BrowseProject(ctx context.Context, in *BrowseProjectRequest, opts ...grpc.CallOption) (*BrowseProjectResponse, error) {
	out := new(BrowseProjectResponse)
	err := c.cc.Invoke(ctx, "/pb.JamsyncAPI/BrowseProject", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *jamsyncAPIClient) UserInfo(ctx context.Context, in *UserInfoRequest, opts ...grpc.CallOption) (*UserInfoResponse, error) {
	out := new(UserInfoResponse)
	err := c.cc.Invoke(ctx, "/pb.JamsyncAPI/UserInfo", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *jamsyncAPIClient) CreateUser(ctx context.Context, in *CreateUserRequest, opts ...grpc.CallOption) (*CreateUserResponse, error) {
	out := new(CreateUserResponse)
	err := c.cc.Invoke(ctx, "/pb.JamsyncAPI/CreateUser", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// JamsyncAPIServer is the server API for JamsyncAPI service.
// All implementations must embed UnimplementedJamsyncAPIServer
// for forward compatibility
type JamsyncAPIServer interface {
	CreateChange(context.Context, *CreateChangeRequest) (*CreateChangeResponse, error)
	WriteOperationStream(JamsyncAPI_WriteOperationStreamServer) error
	CommitChange(context.Context, *CommitChangeRequest) (*CommitChangeResponse, error)
	ReadOperationStream(JamsyncAPI_ReadOperationStreamServer) error
	ReadBlockHashes(context.Context, *ReadBlockHashesRequest) (*ReadBlockHashesResponse, error)
	ReadFile(*ReadFileRequest, JamsyncAPI_ReadFileServer) error
	AddProject(context.Context, *AddProjectRequest) (*AddProjectResponse, error)
	ListProjects(context.Context, *ListProjectsRequest) (*ListProjectsResponse, error)
	BrowseProject(context.Context, *BrowseProjectRequest) (*BrowseProjectResponse, error)
	UserInfo(context.Context, *UserInfoRequest) (*UserInfoResponse, error)
	CreateUser(context.Context, *CreateUserRequest) (*CreateUserResponse, error)
	mustEmbedUnimplementedJamsyncAPIServer()
}

// UnimplementedJamsyncAPIServer must be embedded to have forward compatible implementations.
type UnimplementedJamsyncAPIServer struct {
}

func (UnimplementedJamsyncAPIServer) CreateChange(context.Context, *CreateChangeRequest) (*CreateChangeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreateChange not implemented")
}
func (UnimplementedJamsyncAPIServer) WriteOperationStream(JamsyncAPI_WriteOperationStreamServer) error {
	return status.Errorf(codes.Unimplemented, "method WriteOperationStream not implemented")
}
func (UnimplementedJamsyncAPIServer) CommitChange(context.Context, *CommitChangeRequest) (*CommitChangeResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CommitChange not implemented")
}
func (UnimplementedJamsyncAPIServer) ReadOperationStream(JamsyncAPI_ReadOperationStreamServer) error {
	return status.Errorf(codes.Unimplemented, "method ReadOperationStream not implemented")
}
func (UnimplementedJamsyncAPIServer) ReadBlockHashes(context.Context, *ReadBlockHashesRequest) (*ReadBlockHashesResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ReadBlockHashes not implemented")
}
func (UnimplementedJamsyncAPIServer) ReadFile(*ReadFileRequest, JamsyncAPI_ReadFileServer) error {
	return status.Errorf(codes.Unimplemented, "method ReadFile not implemented")
}
func (UnimplementedJamsyncAPIServer) AddProject(context.Context, *AddProjectRequest) (*AddProjectResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method AddProject not implemented")
}
func (UnimplementedJamsyncAPIServer) ListProjects(context.Context, *ListProjectsRequest) (*ListProjectsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListProjects not implemented")
}
func (UnimplementedJamsyncAPIServer) BrowseProject(context.Context, *BrowseProjectRequest) (*BrowseProjectResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method BrowseProject not implemented")
}
func (UnimplementedJamsyncAPIServer) UserInfo(context.Context, *UserInfoRequest) (*UserInfoResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method UserInfo not implemented")
}
func (UnimplementedJamsyncAPIServer) CreateUser(context.Context, *CreateUserRequest) (*CreateUserResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreateUser not implemented")
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

func _JamsyncAPI_CreateChange_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CreateChangeRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(JamsyncAPIServer).CreateChange(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/pb.JamsyncAPI/CreateChange",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(JamsyncAPIServer).CreateChange(ctx, req.(*CreateChangeRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _JamsyncAPI_WriteOperationStream_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(JamsyncAPIServer).WriteOperationStream(&jamsyncAPIWriteOperationStreamServer{stream})
}

type JamsyncAPI_WriteOperationStreamServer interface {
	Send(*OperationLocation) error
	Recv() (*Operation, error)
	grpc.ServerStream
}

type jamsyncAPIWriteOperationStreamServer struct {
	grpc.ServerStream
}

func (x *jamsyncAPIWriteOperationStreamServer) Send(m *OperationLocation) error {
	return x.ServerStream.SendMsg(m)
}

func (x *jamsyncAPIWriteOperationStreamServer) Recv() (*Operation, error) {
	m := new(Operation)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func _JamsyncAPI_CommitChange_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CommitChangeRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(JamsyncAPIServer).CommitChange(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/pb.JamsyncAPI/CommitChange",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(JamsyncAPIServer).CommitChange(ctx, req.(*CommitChangeRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _JamsyncAPI_ReadOperationStream_Handler(srv interface{}, stream grpc.ServerStream) error {
	return srv.(JamsyncAPIServer).ReadOperationStream(&jamsyncAPIReadOperationStreamServer{stream})
}

type JamsyncAPI_ReadOperationStreamServer interface {
	Send(*Operation) error
	Recv() (*OperationLocation, error)
	grpc.ServerStream
}

type jamsyncAPIReadOperationStreamServer struct {
	grpc.ServerStream
}

func (x *jamsyncAPIReadOperationStreamServer) Send(m *Operation) error {
	return x.ServerStream.SendMsg(m)
}

func (x *jamsyncAPIReadOperationStreamServer) Recv() (*OperationLocation, error) {
	m := new(OperationLocation)
	if err := x.ServerStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

func _JamsyncAPI_ReadBlockHashes_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ReadBlockHashesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(JamsyncAPIServer).ReadBlockHashes(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/pb.JamsyncAPI/ReadBlockHashes",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(JamsyncAPIServer).ReadBlockHashes(ctx, req.(*ReadBlockHashesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _JamsyncAPI_ReadFile_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(ReadFileRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(JamsyncAPIServer).ReadFile(m, &jamsyncAPIReadFileServer{stream})
}

type JamsyncAPI_ReadFileServer interface {
	Send(*Operation) error
	grpc.ServerStream
}

type jamsyncAPIReadFileServer struct {
	grpc.ServerStream
}

func (x *jamsyncAPIReadFileServer) Send(m *Operation) error {
	return x.ServerStream.SendMsg(m)
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
		FullMethod: "/pb.JamsyncAPI/AddProject",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(JamsyncAPIServer).AddProject(ctx, req.(*AddProjectRequest))
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
		FullMethod: "/pb.JamsyncAPI/ListProjects",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(JamsyncAPIServer).ListProjects(ctx, req.(*ListProjectsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _JamsyncAPI_BrowseProject_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(BrowseProjectRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(JamsyncAPIServer).BrowseProject(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/pb.JamsyncAPI/BrowseProject",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(JamsyncAPIServer).BrowseProject(ctx, req.(*BrowseProjectRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _JamsyncAPI_UserInfo_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UserInfoRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(JamsyncAPIServer).UserInfo(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/pb.JamsyncAPI/UserInfo",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(JamsyncAPIServer).UserInfo(ctx, req.(*UserInfoRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _JamsyncAPI_CreateUser_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CreateUserRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(JamsyncAPIServer).CreateUser(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/pb.JamsyncAPI/CreateUser",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(JamsyncAPIServer).CreateUser(ctx, req.(*CreateUserRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// JamsyncAPI_ServiceDesc is the grpc.ServiceDesc for JamsyncAPI service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var JamsyncAPI_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "pb.JamsyncAPI",
	HandlerType: (*JamsyncAPIServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "CreateChange",
			Handler:    _JamsyncAPI_CreateChange_Handler,
		},
		{
			MethodName: "CommitChange",
			Handler:    _JamsyncAPI_CommitChange_Handler,
		},
		{
			MethodName: "ReadBlockHashes",
			Handler:    _JamsyncAPI_ReadBlockHashes_Handler,
		},
		{
			MethodName: "AddProject",
			Handler:    _JamsyncAPI_AddProject_Handler,
		},
		{
			MethodName: "ListProjects",
			Handler:    _JamsyncAPI_ListProjects_Handler,
		},
		{
			MethodName: "BrowseProject",
			Handler:    _JamsyncAPI_BrowseProject_Handler,
		},
		{
			MethodName: "UserInfo",
			Handler:    _JamsyncAPI_UserInfo_Handler,
		},
		{
			MethodName: "CreateUser",
			Handler:    _JamsyncAPI_CreateUser_Handler,
		},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:    "WriteOperationStream",
			Handler:       _JamsyncAPI_WriteOperationStream_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
		{
			StreamName:    "ReadOperationStream",
			Handler:       _JamsyncAPI_ReadOperationStream_Handler,
			ServerStreams: true,
			ClientStreams: true,
		},
		{
			StreamName:    "ReadFile",
			Handler:       _JamsyncAPI_ReadFile_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "pb.proto",
}
