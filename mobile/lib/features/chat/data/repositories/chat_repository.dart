import 'package:dio/dio.dart';
import '../../../../core/constants/api_endpoints.dart';
import '../../../../core/network/api_client.dart';
import '../models/chat_message_model.dart';
import '../models/chat_room_model.dart';

class ChatRepository {
  final ApiClient apiClient;

  ChatRepository({required this.apiClient});

  /// Fetch list of all conversations
  Future<List<ChatRoomModel>> getChatList() async {
    try {
      final response = await apiClient.get(ApiEndpoints.chatList);
      final data = response.data;
      if (data['code'] == 200 && data['body'] != null) {
        final list = data['body'] as List;
        return list.map((item) => ChatRoomModel.fromJson(item)).toList();
      }
      return [];
    } on DioException catch (e) {
      final msg = e.response?.data?['message'] ?? e.message ?? 'Failed to load conversations';
      throw Exception(msg);
    }
  }

  /// Fetch message history for a specific chat
  Future<List<ChatMessageModel>> getChatMessages(String chatId, {int skip = 0, int limit = 30}) async {
    try {
      final response = await apiClient.get(
        '${ApiEndpoints.chatMessages}/$chatId',
        queryParameters: {
          'skip': skip,
          'limit': limit,
        },
      );
      final data = response.data;
      if (data['code'] == 200 && data['body'] != null) {
        final list = data['body'] as List;
        return list.map((item) => ChatMessageModel.fromJson(item)).toList();
      }
      return [];
    } on DioException catch (e) {
      final msg = e.response?.data?['message'] ?? e.message ?? 'Failed to load messages';
      throw Exception(msg);
    }
  }

  /// Send a text message to a user
  Future<ChatMessageModel> sendMessage({
    required String receiverUsername,
    required String text,
  }) async {
    try {
      final response = await apiClient.post(
        '/v1/chat/send',
        data: {
          'username': receiverUsername,
          'text': text,
        },
      );
      final data = response.data;
      if (data['code'] == 200 && data['body'] != null) {
        return ChatMessageModel.fromJson(data['body']);
      }
      throw Exception(data['message'] ?? 'Failed to send message');
    } on DioException catch (e) {
      final msg = e.response?.data?['message'] ?? e.message ?? 'Failed to send message';
      throw Exception(msg);
    }
  }
}
