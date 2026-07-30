package pb

import (
	"context"

	"google.golang.org/grpc"
)

// SendPushRequest message
type SendPushRequest struct {
	ReceiverId string            `json:"receiver_id"`
	Title      string            `json:"title"`
	Body       string            `json:"body"`
	Data       map[string]string `json:"data,omitempty"`
}

// SendPushResponse message
type SendPushResponse struct {
	Success      bool   `json:"success"`
	Message      string `json:"message"`
	SuccessCount int32  `json:"success_count"`
	FailureCount int32  `json:"failure_count"`
}

// FCMGrpcServiceClient is the client API for FCMGrpcService.
type FCMGrpcServiceClient interface {
	SendPushNotification(ctx context.Context, in *SendPushRequest, opts ...grpc.CallOption) (*SendPushResponse, error)
}

type fCMGrpcServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewFCMGrpcServiceClient(cc grpc.ClientConnInterface) FCMGrpcServiceClient {
	return &fCMGrpcServiceClient{cc}
}

func (c *fCMGrpcServiceClient) SendPushNotification(ctx context.Context, in *SendPushRequest, opts ...grpc.CallOption) (*SendPushResponse, error) {
	out := new(SendPushResponse)
	err := c.cc.Invoke(ctx, "/pb.FCMGrpcService/SendPushNotification", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// FCMGrpcServiceServer is the server API for FCMGrpcService service.
type FCMGrpcServiceServer interface {
	SendPushNotification(context.Context, *SendPushRequest) (*SendPushResponse, error)
}

func RegisterFCMGrpcServiceServer(s grpc.ServiceRegistrar, srv FCMGrpcServiceServer) {
	s.RegisterService(&FCMGrpcService_ServiceDesc, srv)
}

func _FCMGrpcService_SendPushNotification_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SendPushRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FCMGrpcServiceServer).SendPushNotification(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/pb.FCMGrpcService/SendPushNotification",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FCMGrpcServiceServer).SendPushNotification(ctx, req.(*SendPushRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var FCMGrpcService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "pb.FCMGrpcService",
	HandlerType: (*FCMGrpcServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "SendPushNotification",
			Handler:    _FCMGrpcService_SendPushNotification_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "pb/fcm.proto",
}
