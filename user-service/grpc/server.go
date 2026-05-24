package grpc

import (
	"context"
	"log"
	"net"
	"social-network-go/pb"
	"social-network-go/user-service/service"

	"google.golang.org/grpc"
)

type GrpcServer struct {
	pb.UserServiceServer
	UserSvc *service.UserService
}

func (s *GrpcServer) GetCommonUserInfo(ctx context.Context, req *pb.UserRequest) (*pb.UserCommonInfoResponse, error) {
	searchKey := req.UserId
	if searchKey == "" {
		searchKey = req.Username
	}

	// Try to lazily ensure profile exists using UserID if we have it
	if req.UserId != "" {
		_, _ = s.UserSvc.EnsureProfile(ctx, req.UserId, "") // Email is unknown here, but EnsureProfile handles it
	}

	// Internal gRPC calls bypass block checks by passing empty currentUserID
	profile, err := s.UserSvc.GetUserProfile(ctx, searchKey, "")
	if err != nil {
		return nil, err
	}

	return &pb.UserCommonInfoResponse{
		UserId:           profile.ID,
		Username:         profile.Username,
		GivenName:        profile.GivenName,
		FamilyName:       profile.FamilyName,
		ProfilePictureId: profile.ProfilePictureId,
		Email:            profile.Email,
	}, nil
}

func (s *GrpcServer) CheckFriendship(ctx context.Context, req *pb.FriendshipRequest) (*pb.FriendshipResponse, error) {
	// Simply fetch target profile and check relationships
	profileA, err := s.UserSvc.GetUserProfile(ctx, req.UserIdA, "")
	if err != nil {
		return nil, err
	}

	friends, _ := s.UserSvc.GetFriends(ctx, profileA.Username, "")
	isFriend := false
	for _, f := range friends {
		if f.ID == req.UserIdB {
			isFriend = true
			break
		}
	}

	blockedUsers, _ := s.UserSvc.GetBlockedUsers(ctx, req.UserIdA)
	hasBlocked := false
	for _, b := range blockedUsers {
		if b.ID == req.UserIdB {
			hasBlocked = true
			break
		}
	}

	sentReqs, _ := s.UserSvc.GetSentRequests(ctx, req.UserIdA)
	hasRequested := false
	for _, r := range sentReqs {
		if r.ID == req.UserIdB {
			hasRequested = true
			break
		}
	}

	return &pb.FriendshipResponse{
		IsFriend:     isFriend,
		HasBlocked:   hasBlocked,
		HasRequested: hasRequested,
	}, nil
}

func (s *GrpcServer) GetUsersByIds(ctx context.Context, req *pb.UsersByIdsRequest) (*pb.UsersByIdsResponse, error) {
	var users []*pb.UserCommonInfoResponse
	for _, id := range req.UserIds {
		profile, err := s.UserSvc.GetUserProfile(ctx, id, "")
		if err == nil {
			users = append(users, &pb.UserCommonInfoResponse{
				UserId:           profile.ID,
				Username:         profile.Username,
				GivenName:        profile.GivenName,
				FamilyName:       profile.FamilyName,
				ProfilePictureId: profile.ProfilePictureId,
				Email:            profile.Email,
			})
		}
	}

	return &pb.UsersByIdsResponse{Users: users}, nil
}

func StartGrpcServer(port string, userSvc *service.UserService) {
	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("Failed to listen on gRPC TCP port %s: %v", port, err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterUserServiceServer(grpcServer, &GrpcServer{UserSvc: userSvc})

	log.Printf("User Service gRPC server listening on port %s", port)
	go func() {
		if err := grpcServer.Serve(lis); err != nil {
			log.Fatalf("Failed to serve gRPC: %v", err)
		}
	}()
}
