import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import '../../../../core/constants/api_endpoints.dart';
import '../../../../core/constants/app_colors.dart';
import '../../../../core/network/api_client.dart';
import '../../../../core/storage/secure_storage_service.dart';
import '../../../../shared/widgets/custom_button.dart';
import '../../../../shared/widgets/user_avatar.dart';
import '../../../auth/presentation/bloc/auth_bloc.dart';
import '../../../feed/presentation/bloc/feed_bloc.dart';

class CreatePostScreen extends StatefulWidget {
  const CreatePostScreen({super.key});

  @override
  State<CreatePostScreen> createState() => _CreatePostScreenState();
}

class _CreatePostScreenState extends State<CreatePostScreen> {
  final _contentController = TextEditingController();
  String _privacy = 'PUBLIC';
  bool _isPublishing = false;

  @override
  void dispose() {
    _contentController.dispose();
    super.dispose();
  }

  Future<void> _publishPost() async {
    final text = _contentController.text.trim();
    if (text.isEmpty || _isPublishing) return;

    setState(() {
      _isPublishing = true;
    });

    try {
      final storage = SecureStorageService();
      final apiClient = ApiClient(storageService: storage);

      final response = await apiClient.post(
        ApiEndpoints.createPost,
        data: {
          'content': text,
          'privacy': _privacy,
          'images': [],
          'files': [],
        },
      );

      final data = response.data;
      if (data['code'] == 200 || data['code'] == 201) {
        if (mounted) {
          context.read<FeedBloc>().add(const LoadFeedEvent(isRefresh: true));
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(
              content: const Row(
                children: [
                  Icon(Icons.check_circle_outline, color: Colors.white, size: 20),
                  SizedBox(width: 8),
                  Text('Post published successfully!'),
                ],
              ),
              backgroundColor: AppColors.success,
              behavior: SnackBarBehavior.floating,
              shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(12)),
            ),
          );
          _contentController.clear();
        }
      } else {
        throw Exception(data['message'] ?? 'Failed to publish post');
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(e.toString().replaceAll('Exception: ', '')),
            backgroundColor: AppColors.error,
            behavior: SnackBarBehavior.floating,
          ),
        );
      }
    } finally {
      if (mounted) {
        setState(() {
          _isPublishing = false;
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final isDark = Theme.of(context).brightness == Brightness.dark;
    final authState = context.watch<AuthBloc>().state;
    String userName = 'You';
    String? userEmail;

    if (authState is AuthenticatedState) {
      userName = authState.session.username;
      userEmail = authState.session.email;
    }

    return Scaffold(
      backgroundColor: isDark ? AppColors.darkBackground : AppColors.lightBackground,
      appBar: AppBar(
        backgroundColor: Colors.transparent,
        elevation: 0,
        title: Text(
          'Create Post',
          style: TextStyle(
            fontSize: 18,
            fontWeight: FontWeight.bold,
            color: isDark ? AppColors.textPrimaryDark : AppColors.textPrimaryLight,
          ),
        ),
        actions: [
          Padding(
            padding: const EdgeInsets.only(right: 16),
            child: Center(
              child: SizedBox(
                height: 36,
                child: CustomButton(
                  text: 'Publish',
                  width: 90,
                  height: 36,
                  isLoading: _isPublishing,
                  onPressed: _publishPost,
                ),
              ),
            ),
          ),
        ],
      ),
      body: SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(18),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              // User header & Privacy selector
              Row(
                children: [
                  UserAvatar(name: userName, radius: 24),
                  const SizedBox(width: 14),
                  Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        userName,
                        style: TextStyle(
                          fontSize: 16,
                          fontWeight: FontWeight.bold,
                          color: isDark ? AppColors.textPrimaryDark : AppColors.textPrimaryLight,
                        ),
                      ),
                      const SizedBox(height: 4),
                      // Privacy selector dropdown pill
                      Container(
                        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 3),
                        decoration: BoxDecoration(
                          color: isDark ? AppColors.darkCard : AppColors.lightCard,
                          borderRadius: BorderRadius.circular(16),
                          border: Border.all(
                            color: isDark ? AppColors.darkBorder : AppColors.lightBorder,
                            width: 0.8,
                          ),
                        ),
                        child: DropdownButtonHideUnderline(
                          child: DropdownButton<String>(
                            value: _privacy,
                            isDense: true,
                            dropdownColor: isDark ? AppColors.darkCard : AppColors.lightCard,
                            icon: const Icon(Icons.arrow_drop_down, size: 18),
                            items: const [
                              DropdownMenuItem(value: 'PUBLIC', child: Text('Public', style: TextStyle(fontSize: 12))),
                              DropdownMenuItem(value: 'FRIEND', child: Text('Friends', style: TextStyle(fontSize: 12))),
                              DropdownMenuItem(value: 'PRIVATE', child: Text('Only Me', style: TextStyle(fontSize: 12))),
                            ],
                            onChanged: (val) {
                              if (val != null) setState(() => _privacy = val);
                            },
                          ),
                        ),
                      ),
                    ],
                  ),
                ],
              ),
              const SizedBox(height: 20),

              // Post Text input area
              TextField(
                controller: _contentController,
                maxLines: 8,
                style: TextStyle(
                  fontSize: 16,
                  height: 1.5,
                  color: isDark ? AppColors.textPrimaryDark : AppColors.textPrimaryLight,
                ),
                decoration: InputDecoration(
                  hintText: "What's on your mind?",
                  hintStyle: TextStyle(
                    fontSize: 16,
                    color: isDark ? AppColors.textTertiaryDark : AppColors.textTertiaryLight,
                  ),
                  border: InputBorder.none,
                ),
              ),
              const SizedBox(height: 30),

              // Media attach options card
              Container(
                padding: const EdgeInsets.all(16),
                decoration: BoxDecoration(
                  color: isDark ? AppColors.darkCard : AppColors.lightCard,
                  borderRadius: BorderRadius.circular(20),
                  border: Border.all(
                    color: isDark ? AppColors.darkBorderSubtle : AppColors.lightBorder,
                  ),
                ),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      'ADD TO YOUR POST',
                      style: TextStyle(
                        fontSize: 12,
                        fontWeight: FontWeight.w700,
                        letterSpacing: 0.4,
                        color: isDark ? AppColors.textSecondaryDark : AppColors.textSecondaryLight,
                      ),
                    ),
                    const SizedBox(height: 14),
                    Row(
                      mainAxisAlignment: MainAxisAlignment.spaceAround,
                      children: [
                        _buildMediaOption(
                          icon: Icons.photo_library_rounded,
                          label: 'Photo',
                          color: AppColors.success,
                          onTap: () {},
                        ),
                        _buildMediaOption(
                          icon: Icons.videocam_rounded,
                          label: 'Video',
                          color: AppColors.primary,
                          onTap: () {},
                        ),
                        _buildMediaOption(
                          icon: Icons.tag_rounded,
                          label: 'Tag',
                          color: AppColors.accent,
                          onTap: () {},
                        ),
                        _buildMediaOption(
                          icon: Icons.location_on_rounded,
                          label: 'Location',
                          color: AppColors.warning,
                          onTap: () {},
                        ),
                      ],
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildMediaOption({
    required IconData icon,
    required String label,
    required Color color,
    required VoidCallback onTap,
  }) {
    return InkWell(
      borderRadius: BorderRadius.circular(14),
      onTap: onTap,
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
        child: Column(
          children: [
            Container(
              padding: const EdgeInsets.all(10),
              decoration: BoxDecoration(
                color: color.withOpacity(0.14),
                shape: BoxShape.circle,
              ),
              child: Icon(icon, color: color, size: 22),
            ),
            const SizedBox(height: 6),
            Text(
              label,
              style: const TextStyle(fontSize: 12, fontWeight: FontWeight.w500),
            ),
          ],
        ),
      ),
    );
  }
}
