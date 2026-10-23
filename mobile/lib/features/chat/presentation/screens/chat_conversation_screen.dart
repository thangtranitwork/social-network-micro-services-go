import 'package:flutter/material.dart';
import '../../../../core/constants/app_colors.dart';
import '../../../../core/network/api_client.dart';
import '../../../../core/storage/secure_storage_service.dart';
import '../../../../shared/widgets/user_avatar.dart';
import '../../data/models/chat_message_model.dart';
import '../../data/models/chat_user_model.dart';
import '../../data/repositories/chat_repository.dart';
import '../../../call/presentation/screens/call_screen.dart';

class ChatConversationScreen extends StatefulWidget {
  final String chatId;
  final ChatUserModel? targetUser;
  final String chatTitle;

  const ChatConversationScreen({
    super.key,
    required this.chatId,
    this.targetUser,
    required this.chatTitle,
  });

  @override
  State<ChatConversationScreen> createState() => _ChatConversationScreenState();
}

class _ChatConversationScreenState extends State<ChatConversationScreen> {
  late final ChatRepository _chatRepository;
  final _messageController = TextEditingController();
  final _scrollController = ScrollController();
  final List<ChatMessageModel> _messages = [];
  bool _isLoading = true;
  bool _isSending = false;
  String _currentUserId = '';

  @override
  void initState() {
    super.initState();
    final storage = SecureStorageService();
    _chatRepository = ChatRepository(apiClient: ApiClient(storageService: storage));
    _initCurrentUserId(storage);
    _loadMessages();
  }

  Future<void> _initCurrentUserId(SecureStorageService storage) async {
    final uid = await storage.getUserId();
    if (mounted) {
      setState(() {
        _currentUserId = uid ?? '';
      });
    }
  }

  @override
  void dispose() {
    _messageController.dispose();
    _scrollController.dispose();
    super.dispose();
  }

  Future<void> _loadMessages() async {
    setState(() => _isLoading = true);
    try {
      final list = await _chatRepository.getChatMessages(widget.chatId, skip: 0, limit: 40);
      setState(() {
        _messages.clear();
        _messages.addAll(list);
        _isLoading = false;
      });
      _scrollToBottom();
    } catch (_) {
      setState(() => _isLoading = false);
    }
  }

  void _scrollToBottom() {
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (_scrollController.hasClients) {
        _scrollController.animateTo(
          _scrollController.position.maxScrollExtent,
          duration: const Duration(milliseconds: 250),
          curve: Curves.easeOut,
        );
      }
    });
  }

  Future<void> _sendMessage() async {
    final text = _messageController.text.trim();
    if (text.isEmpty || _isSending) return;

    final targetUsername = widget.targetUser?.username ?? widget.chatTitle;
    final tempMsg = ChatMessageModel(
      id: DateTime.now().millisecondsSinceEpoch.toString(),
      chatId: widget.chatId,
      senderId: _currentUserId,
      content: text,
      createdAt: DateTime.now(),
    );

    setState(() {
      _messages.add(tempMsg);
      _messageController.clear();
      _isSending = true;
    });
    _scrollToBottom();

    try {
      await _chatRepository.sendMessage(
        receiverUsername: targetUsername,
        text: text,
      );
    } catch (_) {
      // Keep optimistic message or mark error
    } finally {
      if (mounted) {
        setState(() => _isSending = false);
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final isDark = Theme.of(context).brightness == Brightness.dark;

    return Scaffold(
      backgroundColor: isDark ? AppColors.darkBackground : AppColors.lightBackground,
      appBar: AppBar(
        backgroundColor: (isDark ? AppColors.darkCard : AppColors.lightCard).withOpacity(0.95),
        elevation: 0,
        scrolledUnderElevation: 0,
        leading: IconButton(
          icon: Icon(
            Icons.arrow_back_ios_new_rounded,
            size: 20,
            color: isDark ? AppColors.textPrimaryDark : AppColors.textPrimaryLight,
          ),
          onPressed: () => Navigator.pop(context),
        ),
        titleSpacing: 0,
        title: Row(
          children: [
            UserAvatar(
              imageUrl: widget.targetUser?.profilePictureUrl,
              name: widget.chatTitle,
              radius: 18,
              isOnline: widget.targetUser?.isOnline ?? false,
            ),
            const SizedBox(width: 10),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    widget.chatTitle,
                    style: TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.bold,
                      color: isDark ? AppColors.textPrimaryDark : AppColors.textPrimaryLight,
                    ),
                  ),
                  Text(
                    (widget.targetUser?.isOnline ?? false) ? 'Active now' : 'Offline',
                    style: TextStyle(
                      fontSize: 11,
                      color: (widget.targetUser?.isOnline ?? false)
                          ? AppColors.onlineGreen
                          : (isDark ? AppColors.textTertiaryDark : AppColors.textTertiaryLight),
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
        actions: [
          IconButton(
            icon: const Icon(Icons.call_rounded, size: 21, color: AppColors.primary),
            onPressed: () {
              Navigator.push(
                context,
                MaterialPageRoute(
                  builder: (_) => CallScreen(
                    peerName: widget.chatTitle,
                    peerAvatarUrl: widget.targetUser?.profilePictureUrl,
                    callType: CallType.voice,
                  ),
                ),
              );
            },
          ),
          IconButton(
            icon: const Icon(Icons.videocam_rounded, size: 24, color: AppColors.accent),
            onPressed: () {
              Navigator.push(
                context,
                MaterialPageRoute(
                  builder: (_) => CallScreen(
                    peerName: widget.chatTitle,
                    peerAvatarUrl: widget.targetUser?.profilePictureUrl,
                    callType: CallType.video,
                  ),
                ),
              );
            },
          ),
          const SizedBox(width: 6),
        ],
      ),
      body: SafeArea(
        child: Column(
          children: [
            // Messages List
            Expanded(
              child: _isLoading
                  ? const Center(
                      child: CircularProgressIndicator(
                        valueColor: AlwaysStoppedAnimation<Color>(AppColors.primary),
                        strokeWidth: 2.5,
                      ),
                    )
                  : _messages.isEmpty
                      ? Center(
                          child: Column(
                            mainAxisAlignment: MainAxisAlignment.center,
                            children: [
                              UserAvatar(
                                imageUrl: widget.targetUser?.profilePictureUrl,
                                name: widget.chatTitle,
                                radius: 36,
                              ),
                              const SizedBox(height: 12),
                              Text(
                                widget.chatTitle,
                                style: TextStyle(
                                  fontSize: 17,
                                  fontWeight: FontWeight.bold,
                                  color: isDark ? AppColors.textPrimaryDark : AppColors.textPrimaryLight,
                                ),
                              ),
                              const SizedBox(height: 4),
                              Text(
                                'Say hello to start the conversation 👋',
                                style: TextStyle(
                                  fontSize: 13,
                                  color: isDark ? AppColors.textSecondaryDark : AppColors.textSecondaryLight,
                                ),
                              ),
                            ],
                          ),
                        )
                      : ListView.builder(
                          controller: _scrollController,
                          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
                          itemCount: _messages.length,
                          itemBuilder: (context, index) {
                            final msg = _messages[index];
                            final isMe = msg.senderId == _currentUserId;

                            return Align(
                              alignment: isMe ? Alignment.centerRight : Alignment.centerLeft,
                              child: Container(
                                margin: const EdgeInsets.only(bottom: 8),
                                constraints: BoxConstraints(
                                  maxWidth: MediaQuery.of(context).size.width * 0.75,
                                ),
                                padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 11),
                                decoration: BoxDecoration(
                                  gradient: isMe ? AppColors.primaryGradient : null,
                                  color: isMe ? null : (isDark ? AppColors.darkCardElevated : AppColors.lightCard),
                                  borderRadius: BorderRadius.only(
                                    topLeft: const Radius.circular(20),
                                    topRight: const Radius.circular(20),
                                    bottomLeft: Radius.circular(isMe ? 20 : 4),
                                    bottomRight: Radius.circular(isMe ? 4 : 20),
                                  ),
                                  boxShadow: [
                                    BoxShadow(
                                      color: Colors.black.withOpacity(isDark ? 0.2 : 0.04),
                                      blurRadius: 8,
                                      offset: const Offset(0, 2),
                                    ),
                                  ],
                                ),
                                child: Text(
                                  msg.content,
                                  style: TextStyle(
                                    fontSize: 14.5,
                                    height: 1.35,
                                    color: isMe
                                        ? Colors.white
                                        : (isDark ? AppColors.textPrimaryDark : AppColors.textPrimaryLight),
                                  ),
                                ),
                              ),
                            );
                          },
                        ),
            ),

            // Chat Input Bar
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 10),
              decoration: BoxDecoration(
                color: isDark ? AppColors.darkCard : AppColors.lightCard,
                border: Border(
                  top: BorderSide(
                    color: isDark ? AppColors.darkBorderSubtle : AppColors.lightBorder,
                    width: 0.8,
                  ),
                ),
              ),
              child: Row(
                children: [
                  IconButton(
                    icon: Icon(
                      Icons.add_circle_outline_rounded,
                      color: isDark ? AppColors.textSecondaryDark : AppColors.textSecondaryLight,
                      size: 24,
                    ),
                    onPressed: () {},
                  ),
                  Expanded(
                    child: Container(
                      padding: const EdgeInsets.symmetric(horizontal: 14),
                      decoration: BoxDecoration(
                        color: isDark ? AppColors.darkBackground : AppColors.lightBackground,
                        borderRadius: BorderRadius.circular(24),
                      ),
                      child: TextField(
                        controller: _messageController,
                        maxLines: null,
                        style: TextStyle(
                          fontSize: 14.5,
                          color: isDark ? AppColors.textPrimaryDark : AppColors.textPrimaryLight,
                        ),
                        decoration: InputDecoration(
                          hintText: 'Type a message...',
                          hintStyle: TextStyle(
                            fontSize: 14,
                            color: isDark ? AppColors.textTertiaryDark : AppColors.textTertiaryLight,
                          ),
                          border: InputBorder.none,
                          contentPadding: const EdgeInsets.symmetric(vertical: 10),
                        ),
                      ),
                    ),
                  ),
                  const SizedBox(width: 8),
                  GestureDetector(
                    onTap: _sendMessage,
                    child: Container(
                      width: 40,
                      height: 40,
                      decoration: const BoxDecoration(
                        gradient: AppColors.primaryGradient,
                        shape: BoxShape.circle,
                      ),
                      child: const Icon(
                        Icons.send_rounded,
                        color: Colors.white,
                        size: 18,
                      ),
                    ),
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}
