import 'package:flutter/material.dart';

class AppColors {
  // Brand Colors - Electric Violet & Cyber Pink Glow
  static const Color primary = Color(0xFF6366F1); // Indigo
  static const Color primaryDark = Color(0xFF4F46E5);
  static const Color primaryLight = Color(0xFF818CF8);
  static const Color accent = Color(0xFFEC4899); // Neon Pink
  static const Color violetAccent = Color(0xFF8B5CF6);
  static const Color cyanAccent = Color(0xFF06B6D4);

  // High-End OLED Dark Theme (Luxury Native Feel)
  static const Color darkBackground = Color(0xFF08090C); // Deep Midnight Void
  static const Color darkSurface = Color(0xFF0F1017);    // Slightly lifted
  static const Color darkCard = Color(0xFF151620);       // Sleek card container
  static const Color darkCardElevated = Color(0xFF1E2030);
  static const Color darkBorder = Color(0xFF26283B);     // Subtle frosted border
  static const Color darkBorderSubtle = Color(0xFF1A1B28);

  // Modern Clean Light Theme
  static const Color lightBackground = Color(0xFFF9FAFB);
  static const Color lightSurface = Color(0xFFFFFFFF);
  static const Color lightCard = Color(0xFFFFFFFF);
  static const Color lightCardElevated = Color(0xFFF3F4F6);
  static const Color lightBorder = Color(0xFFE5E7EB);

  // Typography Palette
  static const Color textPrimaryDark = Color(0xFFFFFFFF);
  static const Color textSecondaryDark = Color(0xFF9094A6);
  static const Color textTertiaryDark = Color(0xFF5E6278);

  static const Color textPrimaryLight = Color(0xFF111827);
  static const Color textSecondaryLight = Color(0xFF6B7280);
  static const Color textTertiaryLight = Color(0xFF9CA3AF);

  // Status & Live Indicators
  static const Color success = Color(0xFF10B981);
  static const Color error = Color(0xFFF43F5E);
  static const Color warning = Color(0xFFF59E0B);
  static const Color info = Color(0xFF38BDF8);
  static const Color onlineGreen = Color(0xFF10B981);

  // Gradients for High-end Visual Impact
  static const LinearGradient primaryGradient = LinearGradient(
    colors: [Color(0xFF6366F1), Color(0xFF8B5CF6), Color(0xFFEC4899)],
    begin: Alignment.topLeft,
    end: Alignment.bottomRight,
  );

  static const LinearGradient storyGradient = LinearGradient(
    colors: [Color(0xFFFBBF24), Color(0xFFF43F5E), Color(0xFF8B5CF6)],
    begin: Alignment.bottomLeft,
    end: Alignment.topRight,
  );

  static const LinearGradient darkCardGradient = LinearGradient(
    colors: [Color(0xFF161724), Color(0xFF11121C)],
    begin: Alignment.topLeft,
    end: Alignment.bottomRight,
  );
}
