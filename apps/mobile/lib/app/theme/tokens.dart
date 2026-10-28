import 'package:flutter/material.dart';

// The design system (design-system/lottery-platform/MASTER.md) in code. This file and the wheel painter's geometry
// are the only places with raw colours, sizes and durations; widgets read everything through `context.colors`,
// `Elevation`, `Gradients`, `Space`, `Radii` and `Motion`.

/// Semantic colours (MASTER.md §2). Gold is decorative only and never marks an ordinary success. The hero colours
/// exist for two surfaces only: the balance card and the wheel's stage. There is one theme, light, on white.
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
    required this.wheelSegments,
    required this.wheelLucky,
    required this.wheelLabel,
    required this.wheelStar,
    required this.wheelChecker,
    required this.wheelRimDark,
    required this.wheelRimLight,
    required this.shadow,
  });

  /// "Sunset" (MASTER.md §2, owner 2026-10-28): one light theme on a white page. Coral leads, violet accents, gold
  /// stays decorative; status colours all clear 4.5:1 on the page, on cards and on muted panels (tokens_test).
  static const light = AppColors(
    background: Color(0xFFFFFFFF),
    surface: Color(0xFFFFFFFF),
    surfaceRaised: Color(0xFFFFFAF7),
    surfaceMuted: Color(0xFFFBEEE7),
    ink: Color(0xFF2A1A2E),
    inkMuted: Color(0xFF6B5A62),
    primary: Color(0xFFC23412),
    onPrimary: Color(0xFFFFFFFF),
    primaryContainer: Color(0xFFFFE3D8),
    onPrimaryContainer: Color(0xFF7A1F06),
    accent: Color(0xFF6D3FC7),
    gold: Color(0xFFA8781C),
    goldText: Color(0xFF7A5612),
    success: Color(0xFF1B7A43),
    warning: Color(0xFFA05A00),
    error: Color(0xFFC62828),
    pending: Color(0xFF3B5BB5),
    border: Color(0xFFF0DDD5),
    outline: Color(0xFF8A7A80),
    heroStart: Color(0xFFC9381A),
    heroEnd: Color(0xFF3E2585),
    heroGlow: Color(0xFFFF9B6A),
    onHero: Color(0xFFFFFFFF),
    onHeroMuted: Color(0xFFFFE1D2),
    // the wheel is the one place colour runs free (owner, 2026-10-28): six bright hues in turn, a bright gold lucky
    // slot, dark labels on all of them (every pair clears 4.5:1, tokens_test)
    wheelSegments: [
      Color(0xFFFF7A1A), // tangerine
      Color(0xFFFFC53D), // sunflower
      Color(0xFF2DD4BF), // turquoise
      Color(0xFF4CC9FF), // sky
      Color(0xFFB794FF), // lilac
      Color(0xFFFF6FB5), // pink
    ],
    wheelLucky: Color(0xFFFFE066),
    wheelLabel: Color(0xFF2A1A2E),
    wheelStar: Color(0xFF000000),
    wheelChecker: Color(0xFFFFFFFF),
    wheelRimDark: Color(0xFF9A6B10),
    wheelRimLight: Color(0xFFFFE08A),
    shadow: Color(0xFF3C1428),
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

  /// The segments' colours, used in turn around the wheel (the lucky slots take [wheelLucky]).
  final List<Color> wheelSegments;
  final Color wheelLucky;
  final Color wheelLabel;

  /// The lucky slot's star (black, owner 2026-10-28) and the light squares of each segment's inner checker.
  final Color wheelStar;
  final Color wheelChecker;
  final Color wheelRimDark;
  final Color wheelRimLight;
  final Color shadow;
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
  static List<BoxShadow> card(AppColors c) => [
    BoxShadow(color: c.shadow.withValues(alpha: 0.04), blurRadius: 2, offset: const Offset(0, 1)),
    BoxShadow(color: c.shadow.withValues(alpha: 0.06), blurRadius: 24, offset: const Offset(0, 8)),
  ];

  static List<BoxShadow> float(AppColors c) => [
    BoxShadow(color: c.shadow.withValues(alpha: 0.14), blurRadius: 40, offset: const Offset(0, 12)),
  ];

  static List<BoxShadow> hero(AppColors c) => [
    BoxShadow(color: c.heroEnd.withValues(alpha: 0.35), blurRadius: 40, offset: const Offset(0, 16)),
  ];
}

/// The hero surfaces' gradient: the sunset, coral into violet, lit from the top left (MASTER.md §5).
abstract final class Gradients {
  static LinearGradient hero(AppColors c) =>
      LinearGradient(begin: Alignment.topLeft, end: Alignment.bottomRight, colors: [c.heroStart, c.heroEnd]);

  /// The wheel's stage: the dusk end of the sunset, violet into plum, so both segment colours and the gold rim stand
  /// out from it.
  static LinearGradient stage(AppColors c) =>
      LinearGradient(begin: Alignment.topLeft, end: Alignment.bottomRight, colors: [c.heroEnd, c.shadow]);

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
