import 'package:cached_network_image/cached_network_image.dart';
import 'package:flutter/material.dart';
import '../../../../core/constants/app_colors.dart';
import '../../../../core/network/api_client.dart';
import '../../../../core/storage/secure_storage_service.dart';
import '../../../../shared/widgets/user_avatar.dart';
import '../../../profile/presentation/screens/profile_screen.dart';

class ExploreScreen extends StatefulWidget {
  const ExploreScreen({super.key});

  @override
  State<ExploreScreen> createState() => _ExploreScreenState();
}

class _ExploreScreenState extends State<ExploreScreen> {
  final _searchController = TextEditingController();
  List<Map<String, dynamic>> _searchResults = [];
  bool _isSearching = false;
  String? _searchError;

  // Curated high quality explore images
  final List<String> _exploreImages = [
    'https://images.unsplash.com/photo-1517694712202-14dd9538aa97?w=600',
    'https://images.unsplash.com/photo-1618005182384-a83a8bd57fbe?w=600',
    'https://images.unsplash.com/photo-1579783900882-c0d3dad7b119?w=600',
    'https://images.unsplash.com/photo-1550745165-9bc0b252726f?w=600',
    'https://images.unsplash.com/photo-1526374965328-7f61d4dc18c5?w=600',
    'https://images.unsplash.com/photo-1507238691740-187a5b1d37b8?w=600',
    'https://images.unsplash.com/photo-1534972195531-a756b1126f24?w=600',
    'https://images.unsplash.com/photo-1519389950473-47ba0277781c?w=600',
    'https://images.unsplash.com/photo-1518770660439-4636190af475?w=600',
  ];

  @override
  void dispose() {
    _searchController.dispose();
    super.dispose();
  }

  Future<void> _performSearch(String query) async {
    if (query.trim().isEmpty) {
      setState(() {
        _searchResults = [];
        _isSearching = false;
      });
      return;
    }

    setState(() {
      _isSearching = true;
      _searchError = null;
    });

    try {
      final storage = SecureStorageService();
      final apiClient = ApiClient(storageService: storage);

      final response = await apiClient.get('/v1/search?query=${Uri.encodeComponent(query.trim())}');
      final data = response.data;
      if (data['code'] == 200 && data['body'] != null) {
        final list = data['body']['users'] as List? ?? [];
        setState(() {
          _searchResults = list.map((e) => Map<String, dynamic>.from(e)).toList();
          _isSearching = false;
        });
      } else {
        setState(() {
          _searchResults = [];
          _isSearching = false;
        });
      }
    } catch (_) {
      // Fallback: search users from DB via direct username query
      try {
        final storage = SecureStorageService();
        final apiClient = ApiClient(storageService: storage);
        final res = await apiClient.get('/v1/users/${query.trim()}');
        if (res.data['code'] == 200 && res.data['body'] != null) {
          setState(() {
            _searchResults = [Map<String, dynamic>.from(res.data['body'])];
            _isSearching = false;
          });
          return;
        }
      } catch (e2) {
        // No match
      }

      setState(() {
        _searchResults = [];
        _isSearching = false;
      });
    }
  }

  @override
  Widget build(BuildContext context) {
    final isDark = Theme.of(context).brightness == Brightness.dark;
    final isQueryActive = _searchController.text.trim().isNotEmpty;

    return Scaffold(
      backgroundColor: isDark ? AppColors.darkBackground : AppColors.lightBackground,
      appBar: AppBar(
        backgroundColor: Colors.transparent,
        elevation: 0,
        titleSpacing: 16,
        title: Container(
          height: 44,
          decoration: BoxDecoration(
            color: isDark ? AppColors.darkCard : AppColors.lightCard,
            borderRadius: BorderRadius.circular(16),
            border: Border.all(
              color: isDark ? AppColors.darkBorder : AppColors.lightBorder,
              width: 0.8,
            ),
          ),
          child: TextField(
            controller: _searchController,
            onChanged: _performSearch,
            style: TextStyle(
              fontSize: 14.5,
              color: isDark ? AppColors.textPrimaryDark : AppColors.textPrimaryLight,
            ),
            decoration: InputDecoration(
              hintText: 'Search people, tags, topics...',
              hintStyle: TextStyle(
                fontSize: 14,
                color: isDark ? AppColors.textTertiaryDark : AppColors.textTertiaryLight,
              ),
              prefixIcon: Icon(
                Icons.search_rounded,
                size: 20,
                color: isDark ? AppColors.textSecondaryDark : AppColors.textSecondaryLight,
              ),
              suffixIcon: isQueryActive
                  ? IconButton(
                      icon: const Icon(Icons.clear_rounded, size: 18),
                      onPressed: () {
                        _searchController.clear();
                        _performSearch('');
                      },
                    )
                  : null,
              border: InputBorder.none,
              contentPadding: const EdgeInsets.symmetric(vertical: 10),
            ),
          ),
        ),
      ),
      body: isQueryActive
          ? _buildSearchResults(isDark)
          : _buildTrendingExploreGrid(isDark),
    );
  }

  Widget _buildSearchResults(bool isDark) {
    if (_isSearching) {
      return const Center(
        child: CircularProgressIndicator(
          valueColor: AlwaysStoppedAnimation<Color>(AppColors.primary),
          strokeWidth: 2.5,
        ),
      );
    }

    if (_searchResults.isEmpty) {
      return Center(
        child: Column(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            Icon(
              Icons.search_off_rounded,
              size: 52,
              color: isDark ? AppColors.textTertiaryDark : AppColors.textTertiaryLight,
            ),
            const SizedBox(height: 12),
            Text(
              'No users found',
              style: TextStyle(
                fontSize: 15,
                fontWeight: FontWeight.w600,
                color: isDark ? AppColors.textSecondaryDark : AppColors.textSecondaryLight,
              ),
            ),
          ],
        ),
      );
    }

    return ListView.separated(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
      itemCount: _searchResults.length,
      separatorBuilder: (_, __) => Divider(
        height: 1,
        indent: 64,
        color: isDark ? AppColors.darkBorderSubtle : AppColors.lightBorder,
      ),
      itemBuilder: (context, index) {
        final user = _searchResults[index];
        final fullName = '${user["givenName"] ?? ""} ${user["familyName"] ?? ""}'.trim();
        final username = user['username'] ?? '';

        return InkWell(
          borderRadius: BorderRadius.circular(16),
          onTap: () {
            Navigator.push(
              context,
              MaterialPageRoute(
                builder: (_) => ProfileScreen(targetUsername: username),
              ),
            );
          },
          child: Padding(
            padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 12),
            child: Row(
              children: [
                UserAvatar(
                  imageUrl: user['profilePictureUrl'],
                  name: fullName.isNotEmpty ? fullName : username,
                  radius: 24,
                ),
                const SizedBox(width: 14),
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        fullName.isNotEmpty ? fullName : username,
                        style: TextStyle(
                          fontSize: 15,
                          fontWeight: FontWeight.bold,
                          color: isDark ? AppColors.textPrimaryDark : AppColors.textPrimaryLight,
                        ),
                      ),
                      const SizedBox(height: 2),
                      Text(
                        '@$username',
                        style: TextStyle(
                          fontSize: 13,
                          color: isDark ? AppColors.textSecondaryDark : AppColors.textSecondaryLight,
                        ),
                      ),
                    ],
                  ),
                ),
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 6),
                  decoration: BoxDecoration(
                    color: AppColors.primary.withOpacity(0.14),
                    borderRadius: BorderRadius.circular(12),
                  ),
                  child: const Text(
                    'View',
                    style: TextStyle(
                      color: AppColors.primary,
                      fontSize: 13,
                      fontWeight: FontWeight.bold,
                    ),
                  ),
                ),
              ],
            ),
          ),
        );
      },
    );
  }

  Widget _buildTrendingExploreGrid(bool isDark) {
    return GridView.builder(
      padding: const EdgeInsets.fromLTRB(14, 10, 14, 80),
      gridDelegate: const SliverGridDelegateWithFixedCrossAxisCount(
        crossAxisCount: 3,
        crossAxisSpacing: 8,
        mainAxisSpacing: 8,
        childAspectRatio: 1.0,
      ),
      itemCount: _exploreImages.length,
      itemBuilder: (context, index) {
        return ClipRRect(
          borderRadius: BorderRadius.circular(14),
          child: CachedNetworkImage(
            imageUrl: _exploreImages[index],
            fit: BoxFit.cover,
            placeholder: (context, url) => Container(
              color: isDark ? AppColors.darkCard : AppColors.lightCard,
            ),
            errorWidget: (context, url, error) => Container(
              color: isDark ? AppColors.darkCard : AppColors.lightCard,
              child: const Icon(Icons.image, color: Colors.white24),
            ),
          ),
        );
      },
    );
  }
}
