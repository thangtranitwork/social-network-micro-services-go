import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:google_fonts/google_fonts.dart';
import 'core/constants/app_colors.dart';
import 'core/network/api_client.dart';
import 'core/storage/secure_storage_service.dart';
import 'features/auth/data/repositories/auth_repository.dart';
import 'features/auth/presentation/bloc/auth_bloc.dart';
import 'features/auth/presentation/screens/login_screen.dart';
import 'features/feed/data/repositories/feed_repository.dart';
import 'features/feed/presentation/bloc/feed_bloc.dart';
import 'features/navigation/presentation/screens/main_navigation_screen.dart';

void main() async {
  WidgetsFlutterBinding.ensureInitialized();

  // Set immersive edge-to-edge system overlays
  SystemChrome.setSystemUIOverlayStyle(
    const SystemUiOverlayStyle(
      statusBarColor: Colors.transparent,
      statusBarIconBrightness: Brightness.light,
      systemNavigationBarColor: Colors.transparent,
      systemNavigationBarIconBrightness: Brightness.light,
    ),
  );

  final storageService = SecureStorageService();
  final apiClient = ApiClient(storageService: storageService);
  final authRepository = AuthRepository(apiClient: apiClient, storageService: storageService);
  final feedRepository = FeedRepository(apiClient: apiClient);

  runApp(SocialNetworkApp(
    authRepository: authRepository,
    feedRepository: feedRepository,
  ));
}

class SocialNetworkApp extends StatelessWidget {
  final AuthRepository authRepository;
  final FeedRepository feedRepository;

  const SocialNetworkApp({
    super.key,
    required this.authRepository,
    required this.feedRepository,
  });

  @override
  Widget build(BuildContext context) {
    return MultiRepositoryProvider(
      providers: [
        RepositoryProvider.value(value: authRepository),
        RepositoryProvider.value(value: feedRepository),
      ],
      child: MultiBlocProvider(
        providers: [
          BlocProvider(
            create: (context) => AuthBloc(authRepository: authRepository)..add(CheckAuthEvent()),
          ),
          BlocProvider(
            create: (context) => FeedBloc(feedRepository: feedRepository)..add(const LoadFeedEvent()),
          ),
        ],
        child: MaterialApp(
          title: 'PocPoc',
          debugShowCheckedModeBanner: false,
          themeMode: ThemeMode.dark,
          darkTheme: ThemeData(
            brightness: Brightness.dark,
            scaffoldBackgroundColor: AppColors.darkBackground,
            primaryColor: AppColors.primary,
            cardColor: AppColors.darkCard,
            textTheme: GoogleFonts.interTextTheme(
              ThemeData(brightness: Brightness.dark).textTheme,
            ),
            colorScheme: const ColorScheme.dark(
              primary: AppColors.primary,
              secondary: AppColors.accent,
              surface: AppColors.darkCard,
            ),
          ),
          theme: ThemeData(
            brightness: Brightness.light,
            scaffoldBackgroundColor: AppColors.lightBackground,
            primaryColor: AppColors.primary,
            cardColor: AppColors.lightCard,
            textTheme: GoogleFonts.interTextTheme(
              ThemeData(brightness: Brightness.light).textTheme,
            ),
            colorScheme: const ColorScheme.light(
              primary: AppColors.primary,
              secondary: AppColors.accent,
              surface: AppColors.lightCard,
            ),
          ),
          home: const AppRouterView(),
        ),
      ),
    );
  }
}

class AppRouterView extends StatelessWidget {
  const AppRouterView({super.key});

  @override
  Widget build(BuildContext context) {
    return BlocConsumer<AuthBloc, AuthState>(
      listener: (context, state) {
        if (state is AuthenticatedState) {
          // Re-fetch newsfeed for the newly logged-in user
          context.read<FeedBloc>().add(const LoadFeedEvent(isRefresh: true));
        }
      },
      builder: (context, state) {
        if (state is AuthenticatedState) {
          return const MainNavigationScreen();
        }

        if (state is UnauthenticatedState || state is AuthFailureState) {
          return const LoginScreen();
        }

        // Default Splash Loading View
        return Scaffold(
          backgroundColor: AppColors.darkBackground,
          body: Center(
            child: Column(
              mainAxisAlignment: MainAxisAlignment.center,
              children: [
                Container(
                  width: 80,
                  height: 80,
                  decoration: BoxDecoration(
                    gradient: AppColors.primaryGradient,
                    borderRadius: BorderRadius.circular(24),
                    boxShadow: [
                      BoxShadow(
                        color: AppColors.primary.withOpacity(0.4),
                        blurRadius: 30,
                        offset: const Offset(0, 10),
                      ),
                    ],
                  ),
                  child: const Icon(
                    Icons.bubble_chart_rounded,
                    size: 44,
                    color: Colors.white,
                  ),
                ),
                const SizedBox(height: 24),
                Text(
                  'PocPoc',
                  style: GoogleFonts.outfit(
                    fontSize: 32,
                    fontWeight: FontWeight.w800,
                    letterSpacing: -0.5,
                    color: Colors.white,
                  ),
                ),
                const SizedBox(height: 36),
                const SizedBox(
                  width: 24,
                  height: 24,
                  child: CircularProgressIndicator(
                    valueColor: AlwaysStoppedAnimation<Color>(AppColors.primary),
                    strokeWidth: 2.5,
                  ),
                ),
              ],
            ),
          ),
        );
      },
    );
  }
}
