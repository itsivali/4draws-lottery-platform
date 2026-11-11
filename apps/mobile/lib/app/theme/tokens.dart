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
    required this.glass,
    required this.glassBorder,
    required this.glowViolet,
    required this.track,
    required this.selected,
    required this.onSelected,
  });

  /// "Indigo" (MASTER.md §2, owner 2026-11-11, with the 4draws name): one light Material 3 theme. The page is a cool
  /// off-white, cards are white, indigo leads (the logo's own), a lighter indigo accents, and gold stays decorative:
  /// the wheel's rim, the lucky star, the logo's fourth segment. Status colours all clear 4.5:1 on the page, on cards
  /// and on muted panels (tokens_test).
  static const light = AppColors(
    background: Color(0xFFF7F7FB),
    surface: Color(0xFFFFFFFF),
    surfaceRaised: Color(0xFFFBFBFE),
    surfaceMuted: Color(0xFFEFEEF8), // tonal: fields' tracks, quiet chips, the segmented control's track
    ink: Color(0xFF1B1A2E),
    inkMuted: Color(0xFF5A5870),
    primary: Color(0xFF4338CA),
    onPrimary: Color(0xFFFFFFFF),
    primaryContainer: Color(0xFFE4E2FF),
    onPrimaryContainer: Color(0xFF211A78),
    accent: Color(0xFF5B4FE0),
    gold: Color(0xFFB47A12),
    goldText: Color(0xFF7A5200),
    success: Color(0xFF1B7A43),
    warning: Color(0xFF9A5600),
    error: Color(0xFFC62828),
    pending: Color(0xFF0F6A80), // teal, so it is never mistaken for the indigo of an action
    border: Color(0xFFE4E3F0),
    outline: Color(0xFF7C7A93),
    heroStart: Color(0xFF5B4FE9),
    heroEnd: Color(0xFF2A2385),
    heroGlow: Color(0xFF8F88FF),
    onHero: Color(0xFFFFFFFF),
    onHeroMuted: Color(0xFFDDDBFF),
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
    wheelLabel: Color(0xFF1B1A2E),
    wheelStar: Color(0xFF000000),
    wheelChecker: Color(0xFFFFFFFF),
    wheelRimDark: Color(0xFF9A6B10),
    wheelRimLight: Color(0xFFFFE08A),
    shadow: Color(0xFF14123A),
    glass: Color(0x1AFFFFFF), // a tonal wash on a hero surface (the quick actions on the balance)
    glassBorder: Color(0x2EFFFFFF),
    glowViolet: Color(0xFFB7B1FF),
    track: Color(0xFFE6E5F2), // empty bars and ring tracks (decorative, never text)
    selected: Color(0xFFE4E2FF), // Material's selected state: the navigation indicator, a chosen segment or chip
    onSelected: Color(0xFF211A78),
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

  /// A tonal wash over a hero surface, and its edge.
  final Color glass;
  final Color glassBorder;

  /// A soft indigo highlight (the ring gauge's far end).
  final Color glowViolet;

  /// The empty part of a bar or a ring gauge.
  final Color track;

  /// A selected destination, segment or chip: Material's tonal container, and the text and icon on it.
  final Color selected;
  final Color onSelected;
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

/// Corner radii (MASTER.md §5), on Material 3's shape scale: small 8, medium 12, large 16, extra large 28.
abstract final class Radii {
  static const sm = 8.0;
  static const md = 12.0;
  static const lg = 16.0; // banners, panels inside cards
  static const xl = 24.0; // the hero surfaces
  static const xxl = 28.0; // sheets

  /// Text fields and snack bars.
  static const field = 14.0;

  /// Cards.
  static const card = 20.0;
  static const full = 999.0;
}

/// Depth (MASTER.md §5): cards stand clearly off the page, a sheet floats, a hero surface casts its own colour.
abstract final class Elevation {
  // raised again on 2026-11-11 (owner: "cards should stand out with shadows"): a close contact shadow and a wide,
  // soft one in the page's own indigo-black, so white cards lift clearly off the off-white page
  static List<BoxShadow> card(AppColors c) => [
    BoxShadow(color: c.shadow.withValues(alpha: 0.08), blurRadius: 4, offset: const Offset(0, 1)),
    BoxShadow(color: c.shadow.withValues(alpha: 0.13), blurRadius: 32, offset: const Offset(0, 14), spreadRadius: -6),
  ];

  /// A card under the pointer or a finger: it comes forward.
  static List<BoxShadow> cardRaised(AppColors c) => [
    BoxShadow(color: c.shadow.withValues(alpha: 0.08), blurRadius: 6, offset: const Offset(0, 2)),
    BoxShadow(color: c.shadow.withValues(alpha: 0.16), blurRadius: 40, offset: const Offset(0, 20), spreadRadius: -6),
  ];

  /// A bar standing over content.
  static List<BoxShadow> bar(AppColors c) => [
    BoxShadow(color: c.shadow.withValues(alpha: 0.10), blurRadius: 20, offset: const Offset(0, -4)),
  ];

  /// The glow under the selected bar.
  static List<BoxShadow> glow(Color color) => [
    BoxShadow(color: color.withValues(alpha: 0.28), blurRadius: 14, offset: const Offset(0, 5)),
  ];

  static List<BoxShadow> float(AppColors c) => [
    BoxShadow(color: c.shadow.withValues(alpha: 0.18), blurRadius: 32, offset: const Offset(0, 12)),
  ];

  /// The form's card on the signed-out page: deep, over the brand's colour.
  static List<BoxShadow> sheet(AppColors c) => [
    BoxShadow(color: c.shadow.withValues(alpha: 0.22), blurRadius: 12, offset: const Offset(0, 4)),
    BoxShadow(color: c.shadow.withValues(alpha: 0.42), blurRadius: 64, offset: const Offset(0, 32), spreadRadius: -8),
  ];

  static List<BoxShadow> hero(AppColors c) => [
    BoxShadow(color: c.primary.withValues(alpha: 0.18), blurRadius: 6, offset: const Offset(0, 2)),
    BoxShadow(color: c.primary.withValues(alpha: 0.34), blurRadius: 32, offset: const Offset(0, 16), spreadRadius: -4),
  ];
}

/// The hero surfaces' gradient: the logo's indigo, lit from the top left (MASTER.md §5).
abstract final class Gradients {
  static LinearGradient hero(AppColors c) => LinearGradient(
    begin: Alignment.topLeft,
    end: Alignment.bottomRight,
    colors: [c.heroStart, c.primary, c.heroEnd],
    stops: const [0, 0.5, 1],
  );

  /// The wheel's stage: deep indigo into midnight, so the segment colours and the gold rim stand out from it.
  static LinearGradient stage(AppColors c) =>
      LinearGradient(begin: Alignment.topLeft, end: Alignment.bottomRight, colors: [c.heroEnd, c.shadow]);

  /// A soft highlight: [color] fading to nothing.
  static RadialGradient blob(
    Color color, {
    Alignment center = Alignment.center,
    double radius = 0.8,
    double alpha = 0.5,
  }) => RadialGradient(
    center: center,
    radius: radius,
    colors: [
      color.withValues(alpha: alpha),
      color.withValues(alpha: alpha * 0.4),
      color.withValues(alpha: 0),
    ],
    stops: const [0, 0.45, 1],
  );

  /// A filled bar: indigo rising into its lighter accent.
  static LinearGradient bar(AppColors c) =>
      LinearGradient(begin: Alignment.bottomCenter, end: Alignment.topCenter, colors: [c.primary, c.accent]);

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
  static const emphasis = Duration(milliseconds: 380);
  static const stagger = Duration(milliseconds: 45);
  static const skeletonPulse = Duration(milliseconds: 1200);
  static const enter = Curves.easeOutCubic;
  static const exit = Curves.easeInCubic;
  static const settle = Curves.easeOutQuart;
  static const pressScale = 0.98;
  static const riseDistance = 14.0;

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
