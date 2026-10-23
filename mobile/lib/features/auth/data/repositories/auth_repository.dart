import 'dart:convert';
import 'package:dio/dio.dart';
import '../../../../core/constants/api_endpoints.dart';
import '../../../../core/network/api_client.dart';
import '../../../../core/storage/secure_storage_service.dart';
import '../models/user_session_model.dart';

class AuthRepository {
  final ApiClient apiClient;
  final SecureStorageService storageService;

  AuthRepository({
    required this.apiClient,
    required this.storageService,
  });

  /// Login with email/username and password
  Future<UserSessionModel> login({
    required String email,
    required String password,
  }) async {
    try {
      final response = await apiClient.post(
        ApiEndpoints.login,
        data: {
          'email': email,
          'password': password,
        },
      );

      final data = response.data;
      if (data['code'] == 200 && data['body'] != null) {
        final token = data['body']['token'] as String;
        final refreshToken = data['body']['refreshToken'] as String?;

        // Parse payload from JWT token
        final payload = _decodeJwtPayload(token);
        final session = UserSessionModel.fromJson(
          payload,
          token: token,
          refreshToken: refreshToken,
        );

        // Save tokens and session to secure storage
        await storageService.saveToken(token);
        if (refreshToken != null) {
          await storageService.saveRefreshToken(refreshToken);
        }
        await storageService.saveUserId(session.userId);
        await storageService.saveUserInfo(session.toJson());

        return session;
      } else {
        throw Exception(data['message'] ?? 'Login failed');
      }
    } on DioException catch (e) {
      final msg = e.response?.data?['message'] ?? e.message ?? 'Network error during login';
      throw Exception(msg);
    }
  }

  /// Register new user account
  Future<void> register({
    required String email,
    required String username,
    required String password,
    required String givenName,
    required String familyName,
  }) async {
    try {
      final response = await apiClient.post(
        ApiEndpoints.register,
        data: {
          'email': email,
          'username': username,
          'password': password,
          'givenName': givenName,
          'familyName': familyName,
        },
      );

      final data = response.data;
      if (data['code'] != 200 && data['code'] != 201) {
        throw Exception(data['message'] ?? 'Registration failed');
      }
    } on DioException catch (e) {
      final msg = e.response?.data?['message'] ?? e.message ?? 'Network error during registration';
      throw Exception(msg);
    }
  }

  /// Check whether user already has an active session
  Future<UserSessionModel?> getSavedSession() async {
    final token = await storageService.getToken();
    if (token == null || token.isEmpty) {
      return null;
    }

    final userInfo = await storageService.getUserInfo();
    if (userInfo != null) {
      return UserSessionModel.fromJson(userInfo, token: token);
    }

    try {
      final payload = _decodeJwtPayload(token);
      return UserSessionModel.fromJson(payload, token: token);
    } catch (_) {
      return null;
    }
  }

  /// Logout and clear storage
  Future<void> logout() async {
    try {
      await apiClient.post(ApiEndpoints.logout);
    } catch (_) {}
    await storageService.clearAll();
  }

  Map<String, dynamic> _decodeJwtPayload(String token) {
    try {
      final parts = token.split('.');
      if (parts.length != 3) {
        return {};
      }
      final normalized = base64Url.normalize(parts[1]);
      final resp = utf8.decode(base64Url.decode(normalized));
      return json.decode(resp) as Map<String, dynamic>;
    } catch (_) {
      return {};
    }
  }
}
