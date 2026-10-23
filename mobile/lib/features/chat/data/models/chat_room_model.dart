import 'chat_message_model.dart';
import 'chat_user_model.dart';

class ChatRoomModel {
  final String chatId;
  final String name;
  final bool isGroup;
  final String? avatar;
  final ChatUserModel? target;
  final ChatMessageModel? latestMessage;
  final int notReadMessageCount;
  final DateTime updatedAt;

  ChatRoomModel({
    required this.chatId,
    required this.name,
    this.isGroup = false,
    this.avatar,
    this.target,
    this.latestMessage,
    this.notReadMessageCount = 0,
    required this.updatedAt,
  });

  String get title {
    if (isGroup) return name;
    return target?.displayName ?? name;
  }

  factory ChatRoomModel.fromJson(Map<String, dynamic> json) {
    DateTime parsedDate;
    try {
      parsedDate = json['updatedAt'] != null
          ? DateTime.parse(json['updatedAt'])
          : DateTime.now();
    } catch (_) {
      parsedDate = DateTime.now();
    }

    return ChatRoomModel(
      chatId: json['chatId'] ?? json['chat_id'] ?? '',
      name: json['name'] ?? '',
      isGroup: json['isGroup'] ?? false,
      avatar: json['avatar'],
      target: json['target'] != null ? ChatUserModel.fromJson(json['target']) : null,
      latestMessage: json['latestMessage'] != null
          ? ChatMessageModel.fromJson(json['latestMessage'])
          : null,
      notReadMessageCount: json['notReadMessageCount'] ?? 0,
      updatedAt: parsedDate,
    );
  }
}
