package pb

import (
	"context"
	"google.golang.org/grpc"
)

// FileServiceClient is the client API for FileService service.
type FileServiceClient interface {
	DeleteFiles(ctx context.Context, in *DeleteFilesRequest, opts ...grpc.CallOption) (*DeleteFilesResponse, error)
	GetPresignedURL(ctx context.Context, in *GetPresignedURLRequest, opts ...grpc.CallOption) (*GetPresignedURLResponse, error)
	GetPresignedURLs(ctx context.Context, in *GetPresignedURLsRequest, opts ...grpc.CallOption) (*GetPresignedURLsResponse, error)
	GetPresignedUploadURL(ctx context.Context, in *GetPresignedUploadURLRequest, opts ...grpc.CallOption) (*GetPresignedUploadURLResponse, error)
}

type fileServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewFileServiceClient(cc grpc.ClientConnInterface) FileServiceClient {
	return &fileServiceClient{cc}
}

func (c *fileServiceClient) DeleteFiles(ctx context.Context, in *DeleteFilesRequest, opts ...grpc.CallOption) (*DeleteFilesResponse, error) {
	out := new(DeleteFilesResponse)
	err := c.cc.Invoke(ctx, "/pb.FileService/DeleteFiles", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *fileServiceClient) GetPresignedURL(ctx context.Context, in *GetPresignedURLRequest, opts ...grpc.CallOption) (*GetPresignedURLResponse, error) {
	out := new(GetPresignedURLResponse)
	err := c.cc.Invoke(ctx, "/pb.FileService/GetPresignedURL", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *fileServiceClient) GetPresignedURLs(ctx context.Context, in *GetPresignedURLsRequest, opts ...grpc.CallOption) (*GetPresignedURLsResponse, error) {
	out := new(GetPresignedURLsResponse)
	err := c.cc.Invoke(ctx, "/pb.FileService/GetPresignedURLs", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *fileServiceClient) GetPresignedUploadURL(ctx context.Context, in *GetPresignedUploadURLRequest, opts ...grpc.CallOption) (*GetPresignedUploadURLResponse, error) {
	out := new(GetPresignedUploadURLResponse)
	err := c.cc.Invoke(ctx, "/pb.FileService/GetPresignedUploadURL", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// FileServiceServer is the server API for FileService service.
type FileServiceServer interface {
	DeleteFiles(context.Context, *DeleteFilesRequest) (*DeleteFilesResponse, error)
	GetPresignedURL(context.Context, *GetPresignedURLRequest) (*GetPresignedURLResponse, error)
	GetPresignedURLs(context.Context, *GetPresignedURLsRequest) (*GetPresignedURLsResponse, error)
	GetPresignedUploadURL(context.Context, *GetPresignedUploadURLRequest) (*GetPresignedUploadURLResponse, error)
}

type UnimplementedFileServiceServer struct{}

func (UnimplementedFileServiceServer) DeleteFiles(context.Context, *DeleteFilesRequest) (*DeleteFilesResponse, error) {
	return nil, nil
}

func (UnimplementedFileServiceServer) GetPresignedURL(context.Context, *GetPresignedURLRequest) (*GetPresignedURLResponse, error) {
	return nil, nil
}

func (UnimplementedFileServiceServer) GetPresignedURLs(context.Context, *GetPresignedURLsRequest) (*GetPresignedURLsResponse, error) {
	return nil, nil
}

func (UnimplementedFileServiceServer) GetPresignedUploadURL(context.Context, *GetPresignedUploadURLRequest) (*GetPresignedUploadURLResponse, error) {
	return nil, nil
}

func RegisterFileServiceServer(s *grpc.Server, srv FileServiceServer) {
	s.RegisterService(&_FileService_serviceDesc, srv)
}

func _FileService_DeleteFiles_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DeleteFilesRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FileServiceServer).DeleteFiles(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/pb.FileService/DeleteFiles",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FileServiceServer).DeleteFiles(ctx, req.(*DeleteFilesRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _FileService_GetPresignedURL_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetPresignedURLRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FileServiceServer).GetPresignedURL(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/pb.FileService/GetPresignedURL",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FileServiceServer).GetPresignedURL(ctx, req.(*GetPresignedURLRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _FileService_GetPresignedURLs_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetPresignedURLsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FileServiceServer).GetPresignedURLs(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/pb.FileService/GetPresignedURLs",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FileServiceServer).GetPresignedURLs(ctx, req.(*GetPresignedURLsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _FileService_GetPresignedUploadURL_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetPresignedUploadURLRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(FileServiceServer).GetPresignedUploadURL(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/pb.FileService/GetPresignedUploadURL",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(FileServiceServer).GetPresignedUploadURL(ctx, req.(*GetPresignedUploadURLRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _FileService_serviceDesc = grpc.ServiceDesc{
	ServiceName: "pb.FileService",
	HandlerType: (*FileServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "DeleteFiles",
			Handler:    _FileService_DeleteFiles_Handler,
		},
		{
			MethodName: "GetPresignedURL",
			Handler:    _FileService_GetPresignedURL_Handler,
		},
		{
			MethodName: "GetPresignedURLs",
			Handler:    _FileService_GetPresignedURLs_Handler,
		},
		{
			MethodName: "GetPresignedUploadURL",
			Handler:    _FileService_GetPresignedUploadURL_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "pb/file.proto",
}

type DeleteFilesRequest struct {
	FileIds []string `json:"file_ids"`
}

type DeleteFilesResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

type GetPresignedURLRequest struct {
	FileId string `json:"file_id"`
}

type GetPresignedURLResponse struct {
	Url string `json:"url"`
}

type GetPresignedURLsRequest struct {
	FileIds []string `json:"file_ids"`
}

type GetPresignedURLsResponse struct {
	Urls map[string]string `json:"urls"`
}

type GetPresignedUploadURLRequest struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
}

type GetPresignedUploadURLResponse struct {
	FileId string `json:"file_id"`
	Url    string `json:"url"`
}
