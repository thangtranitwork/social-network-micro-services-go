package pb

import (
	"context"
	"google.golang.org/grpc"
)

// UserRequest message
type UserRequest struct {
	UserId   string `json:"user_id"`
	Username string `json:"username"`
}

// UserCommonInfoResponse message
type UserCommonInfoResponse struct {
	UserId           string `json:"user_id"`
	Username         string `json:"username"`
	GivenName        string `json:"given_name"`
	FamilyName       string `json:"family_name"`
	ProfilePictureId string `json:"profile_picture_id"`
	Email            string `json:"email"`
}

// FriendshipRequest message
type FriendshipRequest struct {
	UserIdA string `json:"user_id_a"`
	UserIdB string `json:"user_id_b"`
}

// FriendshipResponse message
type FriendshipResponse struct {
	IsFriend     bool `json:"is_friend"`
	HasBlocked   bool `json:"has_blocked"`
	HasRequested bool `json:"has_requested"`
}

// UsersByIdsRequest message
type UsersByIdsRequest struct {
	UserIds []string `json:"user_ids"`
}

// UsersByIdsResponse message
type UsersByIdsResponse struct {
	Users []*UserCommonInfoResponse `json:"users"`
}

// UserServiceClient is the client API for UserService.
type UserServiceClient interface {
	GetCommonUserInfo(ctx context.Context, in *UserRequest, opts ...grpc.CallOption) (*UserCommonInfoResponse, error)
	CheckFriendship(ctx context.Context, in *FriendshipRequest, opts ...grpc.CallOption) (*FriendshipResponse, error)
	GetUsersByIds(ctx context.Context, in *UsersByIdsRequest, opts ...grpc.CallOption) (*UsersByIdsResponse, error)
}

type userServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewUserServiceClient(cc grpc.ClientConnInterface) UserServiceClient {
	return &userServiceClient{cc}
}

func (c *userServiceClient) GetCommonUserInfo(ctx context.Context, in *UserRequest, opts ...grpc.CallOption) (*UserCommonInfoResponse, error) {
	out := new(UserCommonInfoResponse)
	err := c.cc.Invoke(ctx, "/pb.UserService/GetCommonUserInfo", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *userServiceClient) CheckFriendship(ctx context.Context, in *FriendshipRequest, opts ...grpc.CallOption) (*FriendshipResponse, error) {
	out := new(FriendshipResponse)
	err := c.cc.Invoke(ctx, "/pb.UserService/CheckFriendship", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *userServiceClient) GetUsersByIds(ctx context.Context, in *UsersByIdsRequest, opts ...grpc.CallOption) (*UsersByIdsResponse, error) {
	out := new(UsersByIdsResponse)
	err := c.cc.Invoke(ctx, "/pb.UserService/GetUsersByIds", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// UserServiceServer is the server API for UserService.
type UserServiceServer interface {
	GetCommonUserInfo(context.Context, *UserRequest) (*UserCommonInfoResponse, error)
	CheckFriendship(context.Context, *FriendshipRequest) (*FriendshipResponse, error)
	GetUsersByIds(context.Context, *UsersByIdsRequest) (*UsersByIdsResponse, error)
}

func RegisterUserServiceServer(s grpc.ServiceRegistrar, srv UserServiceServer) {
	s.RegisterService(&UserService_ServiceDesc, srv)
}

func _UserService_GetCommonUserInfo_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UserRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(UserServiceServer).GetCommonUserInfo(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/pb.UserService/GetCommonUserInfo",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(UserServiceServer).GetCommonUserInfo(ctx, req.(*UserRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _UserService_CheckFriendship_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(FriendshipRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(UserServiceServer).CheckFriendship(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/pb.UserService/CheckFriendship",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(UserServiceServer).CheckFriendship(ctx, req.(*FriendshipRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _UserService_GetUsersByIds_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UsersByIdsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(UserServiceServer).GetUsersByIds(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/pb.UserService/GetUsersByIds",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(UserServiceServer).GetUsersByIds(ctx, req.(*UsersByIdsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var UserService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "pb.UserService",
	HandlerType: (*UserServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetCommonUserInfo",
			Handler:    _UserService_GetCommonUserInfo_Handler,
		},
		{
			MethodName: "CheckFriendship",
			Handler:    _UserService_CheckFriendship_Handler,
		},
		{
			MethodName: "GetUsersByIds",
			Handler:    _UserService_GetUsersByIds_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "pb/user.proto",
}
