import 'dart:convert';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

class SecureStorageService {
  static const _storage = FlutterSecureStorage(
    aOptions: AndroidOptions(encryptedSharedPreferences: true),
    iOptions: IOSOptions(accessibility: KeychainAccessibility.first_unlock),
  );

  static const String _keyAccessToken = 'access_token';
  static const String _keyRefreshToken = 'refresh_token';
  static const String _keyUserId = 'user_id';
  static const String _keyUsername = 'username';
  static const String _keyUserInfo = 'user_info';

  Future<void> saveAuthSession({
    required String accessToken,
    required String refreshToken,
    required String userId,
    required String username,
  }) async {
    await Future.wait([
      _storage.write(key: _keyAccessToken, value: accessToken),
      _storage.write(key: _keyRefreshToken, value: refreshToken),
      _storage.write(key: _keyUserId, value: userId),
      _storage.write(key: _keyUsername, value: username),
    ]);
  }

  // Token Methods
  Future<void> saveToken(String token) async => await _storage.write(key: _keyAccessToken, value: token);
  Future<String?> getToken() async => await _storage.read(key: _keyAccessToken);
  Future<String?> getAccessToken() async => await getToken();
  Future<void> updateAccessToken(String newAccessToken) async => await saveToken(newAccessToken);

  // Refresh Token Methods
  Future<void> saveRefreshToken(String token) async => await _storage.write(key: _keyRefreshToken, value: token);
  Future<String?> getRefreshToken() async => await _storage.read(key: _keyRefreshToken);

  // User Id & Username Methods
  Future<void> saveUserId(String userId) async => await _storage.write(key: _keyUserId, value: userId);
  Future<String?> getUserId() async => await _storage.read(key: _keyUserId);
  Future<void> saveUsername(String username) async => await _storage.write(key: _keyUsername, value: username);
  Future<String?> getUsername() async => await _storage.read(key: _keyUsername);

  // User Info JSON Methods
  Future<void> saveUserInfo(Map<String, dynamic> userInfo) async {
    await _storage.write(key: _keyUserInfo, value: jsonEncode(userInfo));
  }

  Future<Map<String, dynamic>?> getUserInfo() async {
    final str = await _storage.read(key: _keyUserInfo);
    if (str == null || str.isEmpty) return null;
    try {
      return jsonDecode(str) as Map<String, dynamic>;
    } catch (_) {
      return null;
    }
  }

  // Cleanup Methods
  Future<void> clearAuthSession() async {
    await Future.wait([
      _storage.delete(key: _keyAccessToken),
      _storage.delete(key: _keyRefreshToken),
      _storage.delete(key: _keyUserId),
      _storage.delete(key: _keyUsername),
      _storage.delete(key: _keyUserInfo),
    ]);
  }

  Future<void> clearAll() async => await clearAuthSession();

  Future<bool> hasValidSession() async {
    final token = await getToken();
    return token != null && token.isNotEmpty;
  }
}
