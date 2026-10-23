import 'package:dio/dio.dart';
import '../../../../core/constants/api_endpoints.dart';
import '../../../../core/network/api_client.dart';
import '../models/comment_model.dart';
import '../models/post_model.dart';

class FeedRepository {
  final ApiClient apiClient;

  FeedRepository({required this.apiClient});

  /// Fetch newsfeed posts with pagination
  Future<List<PostModel>> getNewsfeed({int skip = 0, int limit = 10}) async {
    try {
      final response = await apiClient.get(
        ApiEndpoints.newsfeed,
        queryParameters: {
          'skip': skip,
          'limit': limit,
        },
      );

      final data = response.data;
      if (data['code'] == 200 && data['body'] != null) {
        final list = data['body'] as List;
        return list.map((item) => PostModel.fromJson(item)).toList();
      }
      return [];
    } on DioException catch (e) {
      final msg = e.response?.data?['message'] ?? e.message ?? 'Failed to load newsfeed';
      throw Exception(msg);
    }
  }

  /// Like a post
  Future<void> likePost(String postId) async {
    try {
      await apiClient.post('${ApiEndpoints.likePost}/$postId');
    } on DioException catch (e) {
      final msg = e.response?.data?['message'] ?? e.message ?? 'Failed to like post';
      throw Exception(msg);
    }
  }

  /// Unlike a post
  Future<void> unlikePost(String postId) async {
    try {
      await apiClient.delete('${ApiEndpoints.unlikePost}/$postId');
    } on DioException catch (e) {
      final msg = e.response?.data?['message'] ?? e.message ?? 'Failed to unlike post';
      throw Exception(msg);
    }
  }

  /// Get comments for a post
  Future<List<CommentModel>> getComments(String postId) async {
    try {
      final response = await apiClient.get('${ApiEndpoints.postComments}/$postId');
      final data = response.data;
      if (data['code'] == 200 && data['body'] != null) {
        final list = data['body'] as List;
        return list.map((item) => CommentModel.fromJson(item)).toList();
      }
      return [];
    } on DioException catch (e) {
      final msg = e.response?.data?['message'] ?? e.message ?? 'Failed to load comments';
      throw Exception(msg);
    }
  }

  /// Add comment to post
  Future<CommentModel> addComment({
    required String postId,
    required String content,
  }) async {
    try {
      final response = await apiClient.post(
        ApiEndpoints.createComment,
        data: {
          'postId': postId,
          'content': content,
        },
      );

      final data = response.data;
      if (data['code'] == 200 && data['body'] != null) {
        return CommentModel.fromJson(data['body']);
      }
      throw Exception(data['message'] ?? 'Failed to send comment');
    } on DioException catch (e) {
      final msg = e.response?.data?['message'] ?? e.message ?? 'Failed to post comment';
      throw Exception(msg);
    }
  }
}
