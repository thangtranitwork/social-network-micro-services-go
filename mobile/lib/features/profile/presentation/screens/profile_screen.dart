import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import '../../../../core/constants/api_endpoints.dart';
import '../../../../core/constants/app_colors.dart';
import '../../../../core/network/api_client.dart';
import '../../../../core/storage/secure_storage_service.dart';
import '../../../../shared/widgets/custom_button.dart';
import '../../../../shared/widgets/user_avatar.dart';
import '../../../auth/presentation/bloc/auth_bloc.dart';
import '../../../feed/data/models/post_model.dart';
import '../../../feed/presentation/widgets/post_card.dart';

class ProfileScreen extends StatefulWidget {
  final String? targetUsername;

  const ProfileScreen({super.key, this.targetUsername});

  @override
  State<ProfileScreen> createState() => _ProfileScreenState();
}

class _ProfileScreenState extends State<ProfileScreen> with SingleTickerProviderStateMixin {
  late TabController _tabController;
  Map<String, dynamic>? _userProfile;
  List<PostModel> _userPosts = [];
  bool _isLoading = true;
  String? _errorMessage;

  @override
  void initState() {
    super.initState();
    _tabController = TabController(length: 2, vsync: this);
    _loadProfileData();
  }

  @override
  void dispose() {
    _tabController.dispose();
    super.dispose();
  }

  Future<void> _loadProfileData() async {
    setState(() {
      _isLoading = true;
      _errorMessage = null;
    });

    try {
      final storage = SecureStorageService();
      final apiClient = ApiClient(storageService: storage);

      // Determine username: either passed as prop, or current logged-in user
      String username = widget.targetUsername ?? '';
      if (username.isEmpty) {
        final authState = context.read<AuthBloc>().state;
        if (authState is AuthenticatedState) {
          username = authState.session.username;
        } else {
          username = 'obama';
        }
      }

      // 1. Fetch user profile
      final profileRes = await apiClient.get('/v1/users/$username');
      if (profileRes.data['code'] == 200 && profileRes.data['body'] != null) {
        _userProfile = profileRes.data['body'];
      }

      // 2. Fetch user's posts
      final postsRes = await apiClient.get('/v1/posts/of-user/$username?skip=0&limit=15');
      if (postsRes.data['code'] == 200 && postsRes.data['body'] != null) {
        final list = postsRes.data['body'] as List;
        _userPosts = list.map((item) => PostModel.fromJson(item)).toList();
      }

      setState(() {
        _isLoading = false;
      });
    } catch (e) {
      setState(() {
        _errorMessage = e.toString().replaceAll('Exception: ', '');
        _isLoading = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final isDark = Theme.of(context).brightness == Brightness.dark;

    if (_isLoading) {
      return Scaffold(
        backgroundColor: isDark ? AppColors.darkBackground : AppColors.lightBackground,
        body: const Center(
          child: CircularProgressIndicator(
            valueColor: AlwaysStoppedAnimation<Color>(AppColors.primary),
            strokeWidth: 2.5,
          ),
        ),
      );
    }

    if (_errorMessage != null || _userProfile == null) {
      return Scaffold(
        backgroundColor: isDark ? AppColors.darkBackground : AppColors.lightBackground,
        body: Center(
          child: Column(
            mainAxisAlignment: MainAxisAlignment.center,
            children: [
              Text(
                _errorMessage ?? 'User not found',
                style: const TextStyle(color: AppColors.error),
              ),
              const SizedBox(height: 12),
              ElevatedButton(
                onPressed: _loadProfileData,
                child: const Text('Retry'),
              ),
            ],
          ),
        ),
      );
    }

    final p = _userProfile!;
    final fullName = '${p["givenName"] ?? ""} ${p["familyName"] ?? ""}'.trim();
    final username = p['username'] ?? '';
    final bio = p['bio'] ?? '';
    final friendCount = p['friendCount'] ?? 0;
    final postCount = _userPosts.length;

    return Scaffold(
      backgroundColor: isDark ? AppColors.darkBackground : AppColors.lightBackground,
      appBar: AppBar(
        backgroundColor: Colors.transparent,
        elevation: 0,
        title: Text(
          '@$username',
          style: TextStyle(
            fontSize: 17,
            fontWeight: FontWeight.bold,
            color: isDark ? AppColors.textPrimaryDark : AppColors.textPrimaryLight,
          ),
        ),
        actions: [
          IconButton(
            icon: Icon(
              Icons.logout_rounded,
              color: isDark ? AppColors.textSecondaryDark : AppColors.textSecondaryLight,
            ),
            onPressed: () {
              context.read<AuthBloc>().add(LogoutEvent());
            },
          ),
          const SizedBox(width: 8),
        ],
      ),
      body: NestedScrollView(
        headerSliverBuilder: (context, innerBoxIsScrolled) {
          return [
            SliverToBoxAdapter(
              child: Padding(
                padding: const EdgeInsets.symmetric(horizontal: 20),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    // Profile Header Row (Avatar + Stats)
                    Row(
                      children: [
                        UserAvatar(
                          imageUrl: p['profilePictureUrl'],
                          name: fullName.isNotEmpty ? fullName : username,
                          radius: 40,
                          hasStory: true,
                        ),
                        const SizedBox(width: 24),
                        Expanded(
                          child: Row(
                            mainAxisAlignment: MainAxisAlignment.spaceAround,
                            children: [
                              _buildStatItem('$postCount', 'Posts', isDark),
                              _buildStatItem('$friendCount', 'Friends', isDark),
                              _buildStatItem('0', 'Mutual', isDark),
                            ],
                          ),
                        ),
                      ],
                    ),
                    const SizedBox(height: 16),

                    // Name & Bio
                    Text(
                      fullName.isNotEmpty ? fullName : username,
                      style: TextStyle(
                        fontSize: 18,
                        fontWeight: FontWeight.bold,
                        color: isDark ? AppColors.textPrimaryDark : AppColors.textPrimaryLight,
                      ),
                    ),
                    if (bio.isNotEmpty) ...[
                      const SizedBox(height: 6),
                      Text(
                        bio,
                        style: TextStyle(
                          fontSize: 14,
                          height: 1.4,
                          color: isDark ? AppColors.textSecondaryDark : AppColors.textSecondaryLight,
                        ),
                      ),
                    ],
                    const SizedBox(height: 18),

                    // Action Buttons (Edit Profile, Share Profile)
                    Row(
                      children: [
                        Expanded(
                          child: CustomButton(
                            text: 'Edit Profile',
                            height: 42,
                            isOutlined: true,
                            onPressed: () {},
                          ),
                        ),
                        const SizedBox(width: 12),
                        Container(
                          height: 42,
                          width: 42,
                          decoration: BoxDecoration(
                            color: isDark ? AppColors.darkCard : AppColors.lightCard,
                            borderRadius: BorderRadius.circular(14),
                            border: Border.all(
                              color: isDark ? AppColors.darkBorder : AppColors.lightBorder,
                            ),
                          ),
                          child: const Icon(Icons.share_outlined, size: 20),
                        ),
                      ],
                    ),
                    const SizedBox(height: 20),

                    // Tabs
                    TabBar(
                      controller: _tabController,
                      indicatorColor: AppColors.primary,
                      indicatorWeight: 2.5,
                      labelColor: AppColors.primary,
                      unselectedLabelColor: isDark ? AppColors.textTertiaryDark : AppColors.textTertiaryLight,
                      tabs: const [
                        Tab(icon: Icon(Icons.grid_on_rounded)),
                        Tab(icon: Icon(Icons.video_library_rounded)),
                      ],
                    ),
                  ],
                ),
              ),
            ),
          ];
        },
        body: TabBarView(
          controller: _tabController,
          children: [
            // Tab 1: Posts List
            _userPosts.isEmpty
                ? Center(
                    child: Text(
                      'No posts published yet',
                      style: TextStyle(
                        color: isDark ? AppColors.textSecondaryDark : AppColors.textSecondaryLight,
                      ),
                    ),
                  )
                : ListView.builder(
                    padding: const EdgeInsets.only(top: 8, bottom: 80),
                    itemCount: _userPosts.length,
                    itemBuilder: (context, index) {
                      return PostCard(post: _userPosts[index]);
                    },
                  ),

            // Tab 2: Media Videos
            Center(
              child: Text(
                'No media yet',
                style: TextStyle(
                  color: isDark ? AppColors.textSecondaryDark : AppColors.textSecondaryLight,
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildStatItem(String count, String label, bool isDark) {
    return Column(
      children: [
        Text(
          count,
          style: TextStyle(
            fontSize: 18,
            fontWeight: FontWeight.bold,
            color: isDark ? AppColors.textPrimaryDark : AppColors.textPrimaryLight,
          ),
        ),
        const SizedBox(height: 3),
        Text(
          label,
          style: TextStyle(
            fontSize: 12,
            color: isDark ? AppColors.textSecondaryDark : AppColors.textSecondaryLight,
          ),
        ),
      ],
    );
  }
}
