import 'dart:async';
import 'package:dio/dio.dart';
import '../constants/api_endpoints.dart';
import '../storage/secure_storage_service.dart';

class AuthInterceptor extends QueuedInterceptor {
  final Dio dio;
  final SecureStorageService storageService;
  bool _isRefreshing = false;
  final List<Completer<String?>> _refreshQueue = [];

  AuthInterceptor({
    required this.dio,
    required this.storageService,
  });

  @override
  Future<void> onRequest(
    RequestOptions options,
    RequestInterceptorHandler handler,
  ) async {
    // Skip attaching token for public auth endpoints
    final isPublicEndpoint = options.path.contains('/v1/auth/login') ||
        options.path.contains('/v1/register') ||
        options.path.contains('/v1/auth/forgot-password') ||
        options.path.contains('/v1/auth/reset-password') ||
        options.path.contains('/v1/auth/refresh');

    if (!isPublicEndpoint) {
      final token = await storageService.getAccessToken();
      if (token != null && token.isNotEmpty) {
        options.headers['Authorization'] = 'Bearer $token';
      }
    }

    return handler.next(options);
  }

  @override
  Future<void> onError(
    DioException err,
    ErrorInterceptorHandler handler,
  ) async {
    if (err.response?.statusCode == 401 &&
        !err.requestOptions.path.contains('/v1/auth/login') &&
        !err.requestOptions.path.contains('/v1/auth/refresh')) {
      // 401 Unauthorized detected - Trigger Silent Refresh Token
      final newAccessToken = await _performTokenRefresh();

      if (newAccessToken != null) {
        // Retry original request with newly acquired access token
        final options = err.requestOptions;
        options.headers['Authorization'] = 'Bearer $newAccessToken';

        try {
          final retryResponse = await dio.fetch(options);
          return handler.resolve(retryResponse);
        } catch (retryError) {
          if (retryError is DioException) {
            return handler.next(retryError);
          }
        }
      } else {
        // Refresh token expired or revoked - clear session
        await storageService.clearAuthSession();
      }
    }

    return handler.next(err);
  }

  Future<String?> _performTokenRefresh() async {
    if (_isRefreshing) {
      // If a refresh is already in flight, queue this request
      final completer = Completer<String?>();
      _refreshQueue.add(completer);
      return completer.future;
    }

    _isRefreshing = true;
    try {
      final refreshToken = await storageService.getRefreshToken();
      if (refreshToken == null || refreshToken.isEmpty) {
        _isRefreshing = false;
        _notifyQueue(null);
        return null;
      }

      // Use a clean Dio instance to avoid interceptor recursion
      final cleanDio = Dio(BaseOptions(baseUrl: dio.options.baseUrl));
      final response = await cleanDio.post(
        ApiEndpoints.refreshToken,
        data: {'refreshToken': refreshToken},
      );

      if (response.statusCode == 200 && response.data != null) {
        final newAccessToken = response.data['accessToken'] as String? ??
            response.data['access_token'] as String?;

        if (newAccessToken != null) {
          await storageService.updateAccessToken(newAccessToken);
          _notifyQueue(newAccessToken);
          return newAccessToken;
        }
      }

      _notifyQueue(null);
      return null;
    } catch (_) {
      _notifyQueue(null);
      return null;
    } finally {
      _isRefreshing = false;
    }
  }

  void _notifyQueue(String? token) {
    for (final completer in _refreshQueue) {
      if (!completer.isCompleted) {
        completer.complete(token);
      }
    }
    _refreshQueue.clear();
  }
}
