import 'package:flutter/material.dart';

// The design system (design-system/lottery-platform/MASTER.md) in code. This file and the wheel painter's geometry
// are the only places with raw colours, sizes and durations; widgets read everything through `context.colors`,
// `Elevation`, `Gradients`, `Space`, `Radii` and `Motion`.

/// Semantic colours (MASTER.md §2). Gold is decorative only and never marks an ordinary success. The hero colours
/// exist for two surfaces only: the balance card and the wheel's stage.
@immutable
class AppColors {
  const AppColors({
    required this.background,
    required this.surface,
    required this.surfaceRaised,
    required this.surfaceMuted,
    required this.ink,
    required this.inkMuted,
    required this.primary,
    required this.onPrimary,
    required this.primaryContainer,
    required this.onPrimaryContainer,
    required this.accent,
    required this.gold,
    required this.goldText,
    required this.success,
    required this.warning,
    required this.error,
    required this.pending,
    required this.border,
    required this.outline,
    required this.heroStart,
    required this.heroEnd,
    required this.heroGlow,
    required this.onHero,
    required this.onHeroMuted,
    required this.wheelSegmentA,
    required this.wheelSegmentB,
    required this.wheelLucky,
    required this.wheelLabel,
    required this.wheelRimDark,
    required this.wheelRimLight,
    required this.shadow,
  });

  static const light = AppColors(
    background: Color(0xFFF7F4EC),
    surface: Color(0xFFFFFDF8),
    surfaceRaised: Color(0xFFFFFFFF),
    surfaceMuted: Color(0xFFEFEADF),
    ink: Color(0xFF13201B),
    inkMuted: Color(0xFF56625C),
    primary: Color(0xFF0B5D45),
    onPrimary: Color(0xFFFFFFFF),
    primaryContainer: Color(0xFFD3EBDF),
    onPrimaryContainer: Color(0xFF053B2B),
    accent: Color(0xFF0F6B66),
    gold: Color(0xFF9C7A2E),
    goldText: Color(0xFF7A5B17),
    success: Color(0xFF1B7344),
    warning: Color(0xFF8A5300),
    error: Color(0xFFB3261E),
    pending: Color(0xFF33528F),
    border: Color(0xFFD9D2C2),
    outline: Color(0xFF7C8580),
    heroStart: Color(0xFF0E6A4F),
    heroEnd: Color(0xFF063A2C),
    heroGlow: Color(0xFF3FB58A),
    onHero: Color(0xFFFFFFFF),
    onHeroMuted: Color(0xFFCFE6DB),
    wheelSegmentA: Color(0xFF0B5D45),
    wheelSegmentB: Color(0xFF0E4A3A),
    wheelLucky: Color(0xFF6E5520),
    wheelLabel: Color(0xFFF7F4EC),
    wheelRimDark: Color(0xFF6B5320),
    wheelRimLight: Color(0xFFE2C98A),
    shadow: Color(0xFF13201B),
  );

  static const dark = AppColors(
    background: Color(0xFF0A1310),
    surface: Color(0xFF121E1A),
    surfaceRaised: Color(0xFF182822),
    surfaceMuted: Color(0xFF1C2B26),
    ink: Color(0xFFECF1ED),
    inkMuted: Color(0xFFA3AFA9),
    primary: Color(0xFF62C9A0),
    onPrimary: Color(0xFF00291C),
    primaryContainer: Color(0xFF123D30),
    onPrimaryContainer: Color(0xFFC6EBDB),
    accent: Color(0xFF5CC4BA),
    gold: Color(0xFFD4B36A),
    goldText: Color(0xFFD4B36A),
    success: Color(0xFF72D39C),
    warning: Color(0xFFF0B45B),
    error: Color(0xFFFF8A80),
    pending: Color(0xFF9DB6F2),
    border: Color(0xFF2B3B35),
    outline: Color(0xFF7F8C86),
    heroStart: Color(0xFF145640),
    heroEnd: Color(0xFF0B2E24),
    heroGlow: Color(0xFF62C9A0),
    onHero: Color(0xFFF2F7F3),
    onHeroMuted: Color(0xFFB8D8C8),
    wheelSegmentA: Color(0xFF0E5A43),
    wheelSegmentB: Color(0xFF0A3F31),
    wheelLucky: Color(0xFF6E5520),
    wheelLabel: Color(0xFFF2EEE3),
    wheelRimDark: Color(0xFF7A6130),
    wheelRimLight: Color(0xFFEBD49A),
    shadow: Color(0xFF000000),
  );

  final Color background;
  final Color surface;

  /// Cards that sit on `surface` (sheets' inner panels, the raised row of a list).
  final Color surfaceRaised;
  final Color surfaceMuted;
  final Color ink;
  final Color inkMuted;
  final Color primary;
  final Color onPrimary;
  final Color primaryContainer;
  final Color onPrimaryContainer;
  final Color accent;
  final Color gold;
  final Color goldText;
  final Color success;
  final Color warning;
  final Color error;
  final Color pending;
  final Color border;
  final Color outline;

  /// The two hero surfaces' gradient, its highlight, and the text on them.
  final Color heroStart;
  final Color heroEnd;
  final Color heroGlow;
  final Color onHero;
  final Color onHeroMuted;
  final Color wheelSegmentA;
  final Color wheelSegmentB;
  final Color wheelLucky;
  final Color wheelLabel;
  final Color wheelRimDark;
  final Color wheelRimLight;
  final Color shadow;

  bool get isDark => background.computeLuminance() < 0.2;
}

/// Type faces and the few sizes the theme does not own (MASTER.md §3).
abstract final class AppType {
  /// Headlines, titles and the balance: Manrope, a variable font whose weight the theme sets through FontVariation.
  static const display = 'Manrope';
  static const body = 'IBMPlexSans';
  static const mono = 'IBMPlexMono';
  static const buttonSize = 16.0;
}

/// The 4-point spacing scale (MASTER.md §4).
abstract final class Space {
  static const xxs = 2.0;
  static const xs = 4.0;
  static const sm = 8.0;
  static const md = 12.0;
  static const lg = 16.0;
  static const xl = 24.0;
  static const xxl = 32.0;
  static const xxxl = 48.0;
}

/// Corner radii (MASTER.md §5).
abstract final class Radii {
  static const sm = 8.0;
  static const md = 12.0;
  static const lg = 16.0;
  static const xl = 24.0;
  static const xxl = 28.0;
  static const full = 999.0;
}

/// Depth in three steps (MASTER.md §5): a card's soft lift, a floating sheet, and the hero surfaces' deep shadow.
abstract final class Elevation {
  static List<BoxShadow> card(AppColors c) => c.isDark
      ? [BoxShadow(color: c.shadow.withValues(alpha: 0.35), blurRadius: 24, offset: const Offset(0, 8))]
      : [
          BoxShadow(color: c.shadow.withValues(alpha: 0.04), blurRadius: 2, offset: const Offset(0, 1)),
          BoxShadow(color: c.shadow.withValues(alpha: 0.06), blurRadius: 24, offset: const Offset(0, 8)),
        ];

  static List<BoxShadow> float(AppColors c) => [
    BoxShadow(
      color: c.shadow.withValues(alpha: c.isDark ? 0.55 : 0.14),
      blurRadius: 40,
      offset: const Offset(0, 12),
    ),
  ];

  static List<BoxShadow> hero(AppColors c) => [
    BoxShadow(
      color: c.heroEnd.withValues(alpha: c.isDark ? 0.6 : 0.35),
      blurRadius: 40,
      offset: const Offset(0, 16),
    ),
  ];
}

/// The hero surfaces' gradient: deep emerald, lit from the top left (MASTER.md §5).
abstract final class Gradients {
  static LinearGradient hero(AppColors c) =>
      LinearGradient(begin: Alignment.topLeft, end: Alignment.bottomRight, colors: [c.heroStart, c.heroEnd]);

  static RadialGradient heroGlow(AppColors c) => RadialGradient(
    center: const Alignment(-0.8, -1.2),
    radius: 1.4,
    colors: [c.heroGlow.withValues(alpha: 0.45), c.heroGlow.withValues(alpha: 0)],
  );
}

/// Width classes and content caps (MASTER.md §4).
abstract final class Breakpoints {
  static const medium = 600.0;
  static const expanded = 1024.0;
  static const contentMedium = 720.0;
  static const contentExpanded = 1120.0;
  static const minTouch = 48.0;
  static const buttonHeight = 52.0;
}

/// Motion (MASTER.md §8). The wheel's own timing lives in `SpinTiming`.
abstract final class Motion {
  static const press = Duration(milliseconds: 90);
  static const quick = Duration(milliseconds: 120);
  static const standard = Duration(milliseconds: 220);
  static const emphasis = Duration(milliseconds: 360);
  static const stagger = Duration(milliseconds: 45);
  static const skeletonPulse = Duration(milliseconds: 1200);
  static const enter = Curves.easeOutCubic;
  static const exit = Curves.easeInCubic;
  static const settle = Curves.easeOutQuart;
  static const pressScale = 0.98;
  static const riseDistance = 10.0;

  static Duration exitFor(Duration enter) => enter * 0.65;
}

/// Everything above, reachable from any widget.
@immutable
class AppTokens extends ThemeExtension<AppTokens> {
  const AppTokens(this.colors);

  final AppColors colors;

  @override
  AppTokens copyWith({AppColors? colors}) => AppTokens(colors ?? this.colors);

  @override
  AppTokens lerp(AppTokens? other, double t) => t < 0.5 || other == null ? this : other;
}

extension TokensContext on BuildContext {
  AppColors get colors => Theme.of(this).extension<AppTokens>()!.colors;

  /// Animations are reduced when the system asks (or the player turned them off in the app).
  bool get reduceMotion => MediaQuery.maybeDisableAnimationsOf(this) ?? false;
}
