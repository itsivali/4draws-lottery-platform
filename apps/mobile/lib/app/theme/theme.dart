import 'package:flutter/cupertino.dart' show CupertinoPageTransitionsBuilder;
import 'package:flutter/material.dart';

import 'tokens.dart';

const _tabular = [FontFeature.tabularFigures()];

/// Type scale (MASTER.md §3). Headlines and the balance use the display face, set tighter and heavier; body and labels
/// use the text face. Every style uses tabular figures, so amounts and counts align and never jitter.
TextTheme _textTheme(AppColors c) {
  TextStyle s(double size, double height, FontWeight w, Color color, {bool display = false, double tracking = 0}) =>
      TextStyle(
        fontFamily: display ? AppType.display : AppType.body,
        // the display face is a variable font: its weight axis must be set, FontWeight alone does not move it
        fontVariations: display ? [FontVariation.weight(w.value.toDouble())] : null,
        fontSize: size,
        height: height / size,
        fontWeight: w,
        color: color,
        fontFeatures: _tabular,
        letterSpacing: tracking,
      );
  return TextTheme(
    displayLarge: s(48, 56, FontWeight.w700, c.ink, display: true, tracking: -1.2),
    displayMedium: s(40, 48, FontWeight.w700, c.ink, display: true, tracking: -1.0),
    displaySmall: s(32, 40, FontWeight.w700, c.ink, display: true, tracking: -0.6),
    headlineLarge: s(28, 36, FontWeight.w700, c.ink, display: true, tracking: -0.4),
    headlineMedium: s(28, 36, FontWeight.w600, c.ink, display: true, tracking: -0.4),
    headlineSmall: s(24, 32, FontWeight.w600, c.ink, display: true, tracking: -0.3),
    titleLarge: s(20, 28, FontWeight.w600, c.ink, display: true, tracking: -0.2),
    titleMedium: s(16, 24, FontWeight.w600, c.ink),
    titleSmall: s(14, 20, FontWeight.w600, c.ink),
    bodyLarge: s(16, 24, FontWeight.w400, c.ink),
    bodyMedium: s(16, 24, FontWeight.w400, c.ink),
    bodySmall: s(14, 20, FontWeight.w400, c.inkMuted),
    labelLarge: s(14, 20, FontWeight.w500, c.ink),
    labelMedium: s(13, 18, FontWeight.w500, c.ink),
    labelSmall: s(12, 16, FontWeight.w500, c.inkMuted, tracking: 0.2),
  );
}

ThemeData buildTheme(Brightness brightness) {
  final c = brightness == Brightness.light ? AppColors.light : AppColors.dark;
  final text = _textTheme(c);
  final scheme = ColorScheme(
    brightness: brightness,
    primary: c.primary,
    onPrimary: c.onPrimary,
    primaryContainer: c.primaryContainer,
    onPrimaryContainer: c.onPrimaryContainer,
    secondary: c.accent,
    onSecondary: c.onPrimary,
    tertiary: c.gold,
    onTertiary: c.ink,
    error: c.error,
    onError: c.onPrimary,
    surface: c.surface,
    onSurface: c.ink,
    onSurfaceVariant: c.inkMuted,
    surfaceContainerLowest: c.background,
    surfaceContainerLow: c.surface,
    surfaceContainer: c.surface,
    surfaceContainerHigh: c.surfaceMuted,
    surfaceContainerHighest: c.surfaceMuted,
    outline: c.outline,
    outlineVariant: c.border,
    shadow: c.shadow,
  );
  final buttonShape = RoundedRectangleBorder(borderRadius: BorderRadius.circular(Radii.md));
  const buttonSize = Size(Breakpoints.minTouch, Breakpoints.buttonHeight);
  const buttonPadding = EdgeInsets.symmetric(horizontal: Space.xl);
  final buttonText = text.labelLarge?.copyWith(fontSize: AppType.buttonSize, fontWeight: FontWeight.w600);
  OutlineInputBorder inputBorder(Color color, [double width = 1]) => OutlineInputBorder(
    borderRadius: BorderRadius.circular(Radii.md),
    borderSide: BorderSide(color: color, width: width),
  );

  return ThemeData(
    useMaterial3: true,
    brightness: brightness,
    colorScheme: scheme,
    fontFamily: AppType.body,
    textTheme: text,
    scaffoldBackgroundColor: c.background,
    canvasColor: c.background,
    dividerColor: c.border,
    focusColor: c.primary.withValues(alpha: 0.16),
    splashFactory: InkSparkle.constantTurbulenceSeedSplashFactory,
    extensions: [AppTokens(c)],
    visualDensity: VisualDensity.standard,
    materialTapTargetSize: MaterialTapTargetSize.padded,
    appBarTheme: AppBarTheme(
      backgroundColor: c.background,
      foregroundColor: c.ink,
      surfaceTintColor: Colors.transparent,
      elevation: 0,
      scrolledUnderElevation: 0,
      centerTitle: false,
      titleTextStyle: text.titleLarge,
      titleSpacing: Space.lg,
    ),
    cardTheme: CardThemeData(
      color: c.surface,
      elevation: 0,
      margin: EdgeInsets.zero,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(Radii.lg),
        side: BorderSide(color: c.border),
      ),
    ),
    dividerTheme: DividerThemeData(color: c.border, space: 1, thickness: 1),
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        backgroundColor: c.primary,
        foregroundColor: c.onPrimary,
        disabledBackgroundColor: c.surfaceMuted,
        disabledForegroundColor: c.inkMuted,
        minimumSize: buttonSize,
        padding: buttonPadding,
        shape: buttonShape,
        textStyle: buttonText,
        elevation: 0,
      ),
    ),
    outlinedButtonTheme: OutlinedButtonThemeData(
      style: OutlinedButton.styleFrom(
        foregroundColor: c.primary,
        minimumSize: buttonSize,
        padding: buttonPadding,
        shape: buttonShape,
        side: BorderSide(color: c.outline),
        textStyle: buttonText,
      ),
    ),
    textButtonTheme: TextButtonThemeData(
      style: TextButton.styleFrom(
        foregroundColor: c.primary,
        minimumSize: const Size(Breakpoints.minTouch, Breakpoints.minTouch),
        shape: buttonShape,
        textStyle: text.labelLarge?.copyWith(fontWeight: FontWeight.w600),
      ),
    ),
    iconButtonTheme: IconButtonThemeData(
      style: IconButton.styleFrom(foregroundColor: c.inkMuted, minimumSize: const Size.square(Breakpoints.minTouch)),
    ),
    chipTheme: ChipThemeData(
      backgroundColor: c.surface,
      selectedColor: c.primaryContainer,
      side: BorderSide(color: c.border),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(Radii.full)),
      labelStyle: text.labelLarge,
      secondaryLabelStyle: text.labelLarge?.copyWith(color: c.onPrimaryContainer),
      padding: const EdgeInsets.symmetric(horizontal: Space.md, vertical: Space.sm),
      showCheckmark: false,
    ),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: c.surfaceRaised,
      contentPadding: const EdgeInsets.symmetric(horizontal: Space.lg, vertical: Space.lg),
      border: inputBorder(c.border),
      enabledBorder: inputBorder(c.border),
      focusedBorder: inputBorder(c.primary, 2),
      errorBorder: inputBorder(c.error),
      focusedErrorBorder: inputBorder(c.error, 2),
      labelStyle: text.bodyMedium?.copyWith(color: c.inkMuted),
      floatingLabelStyle: text.bodySmall?.copyWith(color: c.primary),
      helperStyle: text.bodySmall,
      errorStyle: text.bodySmall?.copyWith(color: c.error),
      helperMaxLines: 3,
      errorMaxLines: 3,
    ),
    navigationBarTheme: NavigationBarThemeData(
      backgroundColor: c.surface,
      surfaceTintColor: Colors.transparent,
      indicatorColor: c.primaryContainer,
      indicatorShape: const StadiumBorder(),
      elevation: 0,
      height: 72,
      labelTextStyle: WidgetStateProperty.resolveWith(
        (s) => text.labelMedium?.copyWith(color: s.contains(WidgetState.selected) ? c.ink : c.inkMuted),
      ),
      iconTheme: WidgetStateProperty.resolveWith(
        (s) => IconThemeData(color: s.contains(WidgetState.selected) ? c.onPrimaryContainer : c.inkMuted),
      ),
    ),
    navigationRailTheme: NavigationRailThemeData(
      backgroundColor: c.surface,
      indicatorColor: c.primaryContainer,
      indicatorShape: const StadiumBorder(),
      selectedIconTheme: IconThemeData(color: c.onPrimaryContainer),
      unselectedIconTheme: IconThemeData(color: c.inkMuted),
      selectedLabelTextStyle: text.labelLarge?.copyWith(fontWeight: FontWeight.w600),
      unselectedLabelTextStyle: text.labelLarge?.copyWith(color: c.inkMuted),
    ),
    bottomSheetTheme: BottomSheetThemeData(
      backgroundColor: c.surface,
      surfaceTintColor: Colors.transparent,
      modalBackgroundColor: c.surface,
      showDragHandle: true,
      dragHandleColor: c.outline,
      shape: const RoundedRectangleBorder(borderRadius: BorderRadius.vertical(top: Radius.circular(Radii.xxl))),
    ),
    dialogTheme: DialogThemeData(
      backgroundColor: c.surface,
      surfaceTintColor: Colors.transparent,
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(Radii.xl)),
      titleTextStyle: text.titleLarge,
      contentTextStyle: text.bodyMedium,
    ),
    listTileTheme: ListTileThemeData(
      iconColor: c.inkMuted,
      titleTextStyle: text.bodyLarge,
      subtitleTextStyle: text.bodySmall,
      minVerticalPadding: Space.md,
    ),
    snackBarTheme: SnackBarThemeData(
      behavior: SnackBarBehavior.floating,
      backgroundColor: c.ink,
      contentTextStyle: text.bodyMedium?.copyWith(color: c.background),
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(Radii.md)),
    ),
    segmentedButtonTheme: SegmentedButtonThemeData(
      style: SegmentedButton.styleFrom(
        selectedBackgroundColor: c.primaryContainer,
        selectedForegroundColor: c.onPrimaryContainer,
        side: BorderSide(color: c.border),
        minimumSize: const Size(Breakpoints.minTouch, Breakpoints.minTouch),
      ),
    ),
    progressIndicatorTheme: ProgressIndicatorThemeData(color: c.primary, linearTrackColor: c.surfaceMuted),
    switchTheme: SwitchThemeData(
      thumbColor: WidgetStateProperty.resolveWith((s) => s.contains(WidgetState.selected) ? c.onPrimary : c.outline),
      trackColor: WidgetStateProperty.resolveWith((s) => s.contains(WidgetState.selected) ? c.primary : c.surfaceMuted),
      trackOutlineColor: WidgetStateProperty.all(c.outline),
    ),
    pageTransitionsTheme: const PageTransitionsTheme(
      builders: {
        TargetPlatform.android: FadeForwardsPageTransitionsBuilder(),
        TargetPlatform.iOS: CupertinoPageTransitionsBuilder(),
        TargetPlatform.linux: FadeForwardsPageTransitionsBuilder(),
        TargetPlatform.macOS: FadeForwardsPageTransitionsBuilder(),
        TargetPlatform.windows: FadeForwardsPageTransitionsBuilder(),
      },
    ),
  );
}
