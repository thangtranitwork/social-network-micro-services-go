package pb

import (
	"context"
	"encoding/json"

	"google.golang.org/grpc"
	"google.golang.org/grpc/encoding"
)

func init() {
	// Register a JSON codec so our plain Go structs can be used with gRPC
	// without requiring protobuf code generation
	encoding.RegisterCodec(JSONCodec{})
}

// JSONCodec is a gRPC codec that uses JSON encoding instead of protobuf.
// This allows plain Go structs to be used as gRPC messages.
type JSONCodec struct{}

func (JSONCodec) Marshal(v interface{}) ([]byte, error) {
	return json.Marshal(v)
}

func (JSONCodec) Unmarshal(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

func (JSONCodec) Name() string {
	return "proto" // Override the default "proto" codec with JSON
}

// TokenRequest message
type TokenRequest struct {
	Token string `json:"token"`
}

// TokenResponse message
type TokenResponse struct {
	IsValid      bool   `json:"is_valid"`
	UserId       string `json:"user_id"`
	Email        string `json:"email"`
	Role         string `json:"role"`
	ErrorMessage string `json:"error_message"`
}

// AuthServiceClient is the client API for AuthService.
type AuthServiceClient interface {
	ValidateToken(ctx context.Context, in *TokenRequest, opts ...grpc.CallOption) (*TokenResponse, error)
}

type authServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewAuthServiceClient(cc grpc.ClientConnInterface) AuthServiceClient {
	return &authServiceClient{cc}
}

func (c *authServiceClient) ValidateToken(ctx context.Context, in *TokenRequest, opts ...grpc.CallOption) (*TokenResponse, error) {
	out := new(TokenResponse)
	err := c.cc.Invoke(ctx, "/pb.AuthService/ValidateToken", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// AuthServiceServer is the server API for AuthService.
type AuthServiceServer interface {
	ValidateToken(context.Context, *TokenRequest) (*TokenResponse, error)
}

func RegisterAuthServiceServer(s grpc.ServiceRegistrar, srv AuthServiceServer) {
	s.RegisterService(&AuthService_ServiceDesc, srv)
}

func _AuthService_ValidateToken_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(TokenRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AuthServiceServer).ValidateToken(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/pb.AuthService/ValidateToken",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AuthServiceServer).ValidateToken(ctx, req.(*TokenRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var AuthService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "pb.AuthService",
	HandlerType: (*AuthServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "ValidateToken",
			Handler:    _AuthService_ValidateToken_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "pb/auth.proto",
}
