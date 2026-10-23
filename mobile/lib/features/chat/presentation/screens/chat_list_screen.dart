import 'package:flutter/material.dart';
import 'package:google_fonts/google_fonts.dart';
import '../../../../core/constants/app_colors.dart';
import '../../../../core/network/api_client.dart';
import '../../../../core/storage/secure_storage_service.dart';
import '../../../../shared/widgets/user_avatar.dart';
import '../../data/models/chat_room_model.dart';
import '../../data/repositories/chat_repository.dart';
import 'chat_conversation_screen.dart';

class ChatListScreen extends StatefulWidget {
  const ChatListScreen({super.key});

  @override
  State<ChatListScreen> createState() => _ChatListScreenState();
}

class _ChatListScreenState extends State<ChatListScreen> {
  late final ChatRepository _chatRepository;
  List<ChatRoomModel> _chats = [];
  bool _isLoading = true;
  String? _errorMessage;

  @override
  void initState() {
    super.initState();
    final storage = SecureStorageService();
    _chatRepository = ChatRepository(apiClient: ApiClient(storageService: storage));
    _loadChats();
  }

  Future<void> _loadChats() async {
    setState(() {
      _isLoading = true;
      _errorMessage = null;
    });

    try {
      final list = await _chatRepository.getChatList();
      setState(() {
        _chats = list;
        _isLoading = false;
      });
    } catch (e) {
      setState(() {
        _errorMessage = e.toString().replaceAll('Exception: ', '');
        _isLoading = false;
      });
    }
  }

  String _formatTime(DateTime dateTime) {
    final diff = DateTime.now().difference(dateTime);
    if (diff.inDays > 0) return '${diff.inDays}d';
    if (diff.inHours > 0) return '${diff.inHours}h';
    if (diff.inMinutes > 0) return '${diff.inMinutes}m';
    return 'now';
  }

  @override
  Widget build(BuildContext context) {
    final isDark = Theme.of(context).brightness == Brightness.dark;

    return Scaffold(
      backgroundColor: isDark ? AppColors.darkBackground : AppColors.lightBackground,
      appBar: AppBar(
        backgroundColor: Colors.transparent,
        elevation: 0,
        titleSpacing: 20,
        title: Text(
          'Messages',
          style: GoogleFonts.outfit(
            fontSize: 26,
            fontWeight: FontWeight.w800,
            letterSpacing: -0.5,
            color: isDark ? AppColors.textPrimaryDark : AppColors.textPrimaryLight,
          ),
        ),
        actions: [
          IconButton(
            icon: Icon(
              Icons.edit_square,
              size: 22,
              color: isDark ? AppColors.textPrimaryDark : AppColors.textPrimaryLight,
            ),
            onPressed: () {},
          ),
          const SizedBox(width: 8),
        ],
      ),
      body: _isLoading
          ? const Center(
              child: CircularProgressIndicator(
                valueColor: AlwaysStoppedAnimation<Color>(AppColors.primary),
                strokeWidth: 2.5,
              ),
            )
          : _errorMessage != null
              ? Center(
                  child: Column(
                    mainAxisAlignment: MainAxisAlignment.center,
                    children: [
                      Text(
                        _errorMessage!,
                        style: const TextStyle(color: AppColors.error),
                      ),
                      const SizedBox(height: 12),
                      ElevatedButton(
                        onPressed: _loadChats,
                        child: const Text('Retry'),
                      ),
                    ],
                  ),
                )
              : _chats.isEmpty
                  ? Center(
                      child: Column(
                        mainAxisAlignment: MainAxisAlignment.center,
                        children: [
                          Icon(
                            Icons.chat_bubble_outline_rounded,
                            size: 56,
                            color: isDark ? AppColors.textTertiaryDark : AppColors.textTertiaryLight,
                          ),
                          const SizedBox(height: 14),
                          Text(
                            'No conversations yet',
                            style: TextStyle(
                              fontSize: 16,
                              fontWeight: FontWeight.bold,
                              color: isDark ? AppColors.textPrimaryDark : AppColors.textPrimaryLight,
                            ),
                          ),
                          const SizedBox(height: 6),
                          Text(
                            'Start chatting with your friends',
                            style: TextStyle(
                              fontSize: 13,
                              color: isDark ? AppColors.textSecondaryDark : AppColors.textSecondaryLight,
                            ),
                          ),
                        ],
                      ),
                    )
                  : RefreshIndicator(
                      onRefresh: _loadChats,
                      color: AppColors.primary,
                      child: ListView.separated(
                        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
                        itemCount: _chats.length,
                        separatorBuilder: (_, __) => Divider(
                          height: 1,
                          indent: 64,
                          color: isDark ? AppColors.darkBorderSubtle : AppColors.lightBorder,
                        ),
                        itemBuilder: (context, index) {
                          final chat = _chats[index];
                          final target = chat.target;
                          final title = chat.title;
                          final lastMsg = chat.latestMessage?.content ?? 'Started a conversation';
                          final timeStr = _formatTime(chat.updatedAt);
                          final unread = chat.notReadMessageCount;

                          return InkWell(
                            borderRadius: BorderRadius.circular(16),
                            onTap: () {
                              Navigator.push(
                                context,
                                MaterialPageRoute(
                                  builder: (_) => ChatConversationScreen(
                                    chatId: chat.chatId,
                                    targetUser: target,
                                    chatTitle: title,
                                  ),
                                ),
                              ).then((_) => _loadChats());
                            },
                            child: Padding(
                              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 12),
                              child: Row(
                                children: [
                                  UserAvatar(
                                    imageUrl: target?.profilePictureUrl ?? chat.avatar,
                                    name: title,
                                    radius: 26,
                                    isOnline: target?.isOnline ?? false,
                                  ),
                                  const SizedBox(width: 14),
                                  Expanded(
                                    child: Column(
                                      crossAxisAlignment: CrossAxisAlignment.start,
                                      children: [
                                        Text(
                                          title,
                                          style: TextStyle(
                                            fontSize: 15,
                                            fontWeight: FontWeight.w700,
                                            color: isDark ? AppColors.textPrimaryDark : AppColors.textPrimaryLight,
                                          ),
                                        ),
                                        const SizedBox(height: 4),
                                        Text(
                                          lastMsg,
                                          maxLines: 1,
                                          overflow: TextOverflow.ellipsis,
                                          style: TextStyle(
                                            fontSize: 13,
                                            fontWeight: unread > 0 ? FontWeight.w600 : FontWeight.normal,
                                            color: unread > 0
                                                ? (isDark ? AppColors.textPrimaryDark : AppColors.textPrimaryLight)
                                                : (isDark ? AppColors.textSecondaryDark : AppColors.textSecondaryLight),
                                          ),
                                        ),
                                      ],
                                    ),
                                  ),
                                  const SizedBox(width: 8),
                                  Column(
                                    crossAxisAlignment: CrossAxisAlignment.end,
                                    children: [
                                      Text(
                                        timeStr,
                                        style: TextStyle(
                                          fontSize: 11,
                                          color: unread > 0 ? AppColors.primary : (isDark ? AppColors.textTertiaryDark : AppColors.textTertiaryLight),
                                        ),
                                      ),
                                      const SizedBox(height: 6),
                                      if (unread > 0)
                                        Container(
                                          padding: const EdgeInsets.symmetric(horizontal: 7, vertical: 3),
                                          decoration: BoxDecoration(
                                            gradient: AppColors.primaryGradient,
                                            borderRadius: BorderRadius.circular(10),
                                          ),
                                          child: Text(
                                            '$unread',
                                            style: const TextStyle(
                                              color: Colors.white,
                                              fontSize: 11,
                                              fontWeight: FontWeight.bold,
                                            ),
                                          ),
                                        ),
                                    ],
                                  ),
                                ],
                              ),
                            ),
                          );
                        },
                      ),
                    ),
    );
  }
}
