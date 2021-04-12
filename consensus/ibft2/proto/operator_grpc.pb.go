// Code generated by protoc-gen-go-grpc. DO NOT EDIT.

package proto

import (
	context "context"
	empty "github.com/golang/protobuf/ptypes/empty"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// OperatorClient is the client API for Operator service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type OperatorClient interface {
	GetSnapshot(ctx context.Context, in *SnapshotReq, opts ...grpc.CallOption) (*Snapshot, error)
	Propose(ctx context.Context, in *Candidate, opts ...grpc.CallOption) (*empty.Empty, error)
	Candidates(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (*CandidatesResp, error)
}

type operatorClient struct {
	cc grpc.ClientConnInterface
}

func NewOperatorClient(cc grpc.ClientConnInterface) OperatorClient {
	return &operatorClient{cc}
}

func (c *operatorClient) GetSnapshot(ctx context.Context, in *SnapshotReq, opts ...grpc.CallOption) (*Snapshot, error) {
	out := new(Snapshot)
	err := c.cc.Invoke(ctx, "/v1.Operator/GetSnapshot", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *operatorClient) Propose(ctx context.Context, in *Candidate, opts ...grpc.CallOption) (*empty.Empty, error) {
	out := new(empty.Empty)
	err := c.cc.Invoke(ctx, "/v1.Operator/Propose", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *operatorClient) Candidates(ctx context.Context, in *empty.Empty, opts ...grpc.CallOption) (*CandidatesResp, error) {
	out := new(CandidatesResp)
	err := c.cc.Invoke(ctx, "/v1.Operator/Candidates", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// OperatorServer is the server API for Operator service.
// All implementations must embed UnimplementedOperatorServer
// for forward compatibility
type OperatorServer interface {
	GetSnapshot(context.Context, *SnapshotReq) (*Snapshot, error)
	Propose(context.Context, *Candidate) (*empty.Empty, error)
	Candidates(context.Context, *empty.Empty) (*CandidatesResp, error)
	mustEmbedUnimplementedOperatorServer()
}

// UnimplementedOperatorServer must be embedded to have forward compatible implementations.
type UnimplementedOperatorServer struct {
}

func (UnimplementedOperatorServer) GetSnapshot(context.Context, *SnapshotReq) (*Snapshot, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetSnapshot not implemented")
}
func (UnimplementedOperatorServer) Propose(context.Context, *Candidate) (*empty.Empty, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Propose not implemented")
}
func (UnimplementedOperatorServer) Candidates(context.Context, *empty.Empty) (*CandidatesResp, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Candidates not implemented")
}
func (UnimplementedOperatorServer) mustEmbedUnimplementedOperatorServer() {}

// UnsafeOperatorServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to OperatorServer will
// result in compilation errors.
type UnsafeOperatorServer interface {
	mustEmbedUnimplementedOperatorServer()
}

func RegisterOperatorServer(s grpc.ServiceRegistrar, srv OperatorServer) {
	s.RegisterService(&Operator_ServiceDesc, srv)
}

func _Operator_GetSnapshot_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SnapshotReq)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(OperatorServer).GetSnapshot(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/v1.Operator/GetSnapshot",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(OperatorServer).GetSnapshot(ctx, req.(*SnapshotReq))
	}
	return interceptor(ctx, in, info, handler)
}

func _Operator_Propose_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(Candidate)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(OperatorServer).Propose(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/v1.Operator/Propose",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(OperatorServer).Propose(ctx, req.(*Candidate))
	}
	return interceptor(ctx, in, info, handler)
}

func _Operator_Candidates_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(empty.Empty)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(OperatorServer).Candidates(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/v1.Operator/Candidates",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(OperatorServer).Candidates(ctx, req.(*empty.Empty))
	}
	return interceptor(ctx, in, info, handler)
}

// Operator_ServiceDesc is the grpc.ServiceDesc for Operator service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Operator_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "v1.Operator",
	HandlerType: (*OperatorServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetSnapshot",
			Handler:    _Operator_GetSnapshot_Handler,
		},
		{
			MethodName: "Propose",
			Handler:    _Operator_Propose_Handler,
		},
		{
			MethodName: "Candidates",
			Handler:    _Operator_Candidates_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "consensus/ibft2/proto/operator.proto",
}
