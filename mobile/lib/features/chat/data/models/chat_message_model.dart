class ChatMessageModel {
  final String id;
  final String chatId;
  final String senderId;
  final String content;
  final String type; // TEXT, FILE, GIF, VOICE
  final DateTime createdAt;
  final bool isRead;

  ChatMessageModel({
    required this.id,
    required this.chatId,
    required this.senderId,
    required this.content,
    this.type = 'TEXT',
    required this.createdAt,
    this.isRead = false,
  });

  factory ChatMessageModel.fromJson(Map<String, dynamic> json) {
    DateTime parsedDate;
    try {
      parsedDate = json['createdAt'] != null
          ? DateTime.parse(json['createdAt'])
          : DateTime.now();
    } catch (_) {
      parsedDate = DateTime.now();
    }

    return ChatMessageModel(
      id: json['id'] ?? json['messageId'] ?? '',
      chatId: json['chatId'] ?? json['chat_id'] ?? '',
      senderId: json['senderId'] ?? json['sender_id'] ?? '',
      content: json['content'] ?? json['text'] ?? '',
      type: json['type'] ?? 'TEXT',
      createdAt: parsedDate,
      isRead: json['isRead'] ?? false,
    );
  }
}
