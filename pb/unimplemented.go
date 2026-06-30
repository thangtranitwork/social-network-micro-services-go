package pb

import (
	"context"
	"errors"
)

// UnimplementedAuthServiceServer can be embedded to have forward compatible implementations.
type UnimplementedAuthServiceServer struct{}

func (UnimplementedAuthServiceServer) ValidateToken(ctx context.Context, req *TokenRequest) (*TokenResponse, error) {
	return nil, errors.New("method ValidateToken not implemented")
}

// UnimplementedUserServiceServer can be embedded to have forward compatible implementations.
type UnimplementedUserServiceServer struct{}

func (UnimplementedUserServiceServer) GetCommonUserInfo(ctx context.Context, req *UserRequest) (*UserCommonInfoResponse, error) {
	return nil, errors.New("method GetCommonUserInfo not implemented")
}

func (UnimplementedUserServiceServer) CheckFriendship(ctx context.Context, req *FriendshipRequest) (*FriendshipResponse, error) {
	return nil, errors.New("method CheckFriendship not implemented")
}

func (UnimplementedUserServiceServer) GetUsersByIds(ctx context.Context, req *UsersByIdsRequest) (*UsersByIdsResponse, error) {
	return nil, errors.New("method GetUsersByIds not implemented")
}

func (UnimplementedUserServiceServer) CreateUser(ctx context.Context, req *CreateUserRequest) (*CreateUserResponse, error) {
	return nil, errors.New("method CreateUser not implemented")
}
