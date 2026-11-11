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
    // big numbers: medium weight and tightly tracked, a small muted caption over them
    displayLarge: s(52, 60, FontWeight.w600, c.ink, display: true, tracking: -1.6),
    displayMedium: s(40, 48, FontWeight.w600, c.ink, display: true, tracking: -1.0),
    displaySmall: s(34, 42, FontWeight.w500, c.ink, display: true, tracking: -1.0),
    headlineLarge: s(30, 38, FontWeight.w600, c.ink, display: true, tracking: -0.9),
    headlineMedium: s(28, 36, FontWeight.w500, c.ink, display: true, tracking: -0.8),
    headlineSmall: s(24, 32, FontWeight.w500, c.ink, display: true, tracking: -0.6),
    titleLarge: s(22, 30, FontWeight.w600, c.ink, display: true, tracking: -0.4),
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

/// A display style at another weight: the display face is variable, so its weight axis moves with the weight.
extension DisplayWeight on TextStyle {
  TextStyle weighted(FontWeight w) => copyWith(
    fontWeight: w,
    fontVariations: fontFamily == AppType.display ? [FontVariation.weight(w.value.toDouble())] : fontVariations,
  );
}

/// The app's one theme: light Material 3 in the 4draws indigo (MASTER.md §12).
ThemeData buildTheme() {
  const c = AppColors.light;
  final text = _textTheme(c);
  final scheme = ColorScheme(
    brightness: Brightness.light,
    primary: c.primary,
    onPrimary: c.onPrimary,
    primaryContainer: c.primaryContainer,
    onPrimaryContainer: c.onPrimaryContainer,
    secondary: c.accent,
    onSecondary: c.onPrimary,
    // tonal buttons, the navigation indicator and chosen segments share one container (Material 3)
    secondaryContainer: c.selected,
    onSecondaryContainer: c.onSelected,
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
  // Material 3: buttons are fully rounded
  const buttonShape = StadiumBorder();
  const buttonSize = Size(Breakpoints.minTouch, Breakpoints.buttonHeight);
  const buttonPadding = EdgeInsets.symmetric(horizontal: Space.xl);
  final buttonText = text.labelLarge?.copyWith(fontSize: AppType.buttonSize, fontWeight: FontWeight.w600);
  OutlineInputBorder inputBorder(Color color, [double width = 1]) => OutlineInputBorder(
    borderRadius: BorderRadius.circular(Radii.field),
    borderSide: BorderSide(color: color, width: width),
  );

  return ThemeData(
    useMaterial3: true,
    brightness: Brightness.light,
    colorScheme: scheme,
    fontFamily: AppType.body,
    textTheme: text,
    scaffoldBackgroundColor: c.background,
    canvasColor: c.background,
    dividerColor: c.border,
    focusColor: c.primary.withValues(alpha: 0.16),
    splashFactory: InkSparkle.constantTurbulenceSeedSplashFactory,
    extensions: const [AppTokens(c)],
    visualDensity: VisualDensity.standard,
    materialTapTargetSize: MaterialTapTargetSize.padded,
    appBarTheme: AppBarTheme(
      backgroundColor: c.background,
      foregroundColor: c.ink,
      surfaceTintColor: Colors.transparent,
      elevation: 0,
      // Material 3: the bar lifts a little once content scrolls under it
      scrolledUnderElevation: 3,
      shadowColor: c.shadow.withValues(alpha: 0.35),
      centerTitle: false,
      titleTextStyle: text.titleLarge,
      titleSpacing: Space.lg,
      toolbarHeight: 64,
    ),
    cardTheme: CardThemeData(
      color: c.surface,
      elevation: 0,
      margin: EdgeInsets.zero,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(Radii.card),
        side: BorderSide(color: c.border),
      ),
    ),
    dividerTheme: DividerThemeData(color: c.border, space: 1, thickness: 1),
    // Material 3 buttons: a flat filled pill for the one primary action, an outlined pill beside it, tonal for the
    // quiet ones. No gradients, no gloss: the state layer and the ripple are the feedback, as on the platform.
    filledButtonTheme: FilledButtonThemeData(
      style: FilledButton.styleFrom(
        backgroundColor: c.primary,
        foregroundColor: c.onPrimary,
        disabledBackgroundColor: c.ink.withValues(alpha: 0.10),
        disabledForegroundColor: c.ink.withValues(alpha: 0.42),
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
      style: IconButton.styleFrom(foregroundColor: c.ink, minimumSize: const Size.square(Breakpoints.minTouch)),
    ),
    // chips: outlined, or filled with the tonal container when chosen (Material 3)
    chipTheme: ChipThemeData(
      backgroundColor: c.surface,
      selectedColor: c.selected,
      disabledColor: c.surfaceMuted,
      side: WidgetStateBorderSide.resolveWith(
        (s) => BorderSide(color: s.contains(WidgetState.selected) ? c.selected : c.border),
      ),
      shape: const StadiumBorder(),
      labelStyle: text.labelLarge?.copyWith(fontWeight: FontWeight.w600),
      secondaryLabelStyle: text.labelLarge?.copyWith(color: c.onSelected, fontWeight: FontWeight.w600),
      padding: const EdgeInsets.symmetric(horizontal: Space.md, vertical: Space.sm),
      showCheckmark: false,
      elevation: 0,
      pressElevation: 0,
    ),
    inputDecorationTheme: InputDecorationTheme(
      filled: true,
      fillColor: c.surface,
      contentPadding: const EdgeInsets.symmetric(horizontal: Space.lg, vertical: Space.lg),
      border: inputBorder(c.outline),
      enabledBorder: inputBorder(c.outline),
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
    // Material 3 navigation: a full-width bar on the surface, the chosen destination under a tonal pill
    navigationBarTheme: NavigationBarThemeData(
      backgroundColor: c.surface,
      surfaceTintColor: Colors.transparent,
      indicatorColor: c.selected,
      indicatorShape: const StadiumBorder(),
      elevation: 0,
      height: 72,
      labelBehavior: NavigationDestinationLabelBehavior.alwaysShow,
      labelTextStyle: WidgetStateProperty.resolveWith(
        (s) => s.contains(WidgetState.selected)
            ? text.labelMedium?.copyWith(color: c.ink, fontWeight: FontWeight.w700)
            : text.labelMedium?.copyWith(color: c.inkMuted),
      ),
      iconTheme: WidgetStateProperty.resolveWith(
        (s) => IconThemeData(color: s.contains(WidgetState.selected) ? c.onSelected : c.inkMuted, size: 24),
      ),
    ),
    navigationRailTheme: NavigationRailThemeData(
      backgroundColor: c.surface,
      indicatorColor: c.selected,
      indicatorShape: const StadiumBorder(),
      selectedIconTheme: IconThemeData(color: c.onSelected),
      unselectedIconTheme: IconThemeData(color: c.inkMuted),
      selectedLabelTextStyle: text.labelLarge?.copyWith(fontWeight: FontWeight.w700),
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
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(Radii.card)),
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
      shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(Radii.lg)),
    ),
    // Material 3's segmented button: outlined, the chosen segment tonal
    segmentedButtonTheme: SegmentedButtonThemeData(
      style: SegmentedButton.styleFrom(
        foregroundColor: c.ink,
        selectedBackgroundColor: c.selected,
        selectedForegroundColor: c.onSelected,
        side: BorderSide(color: c.outline),
        shape: const StadiumBorder(),
        textStyle: text.labelLarge?.copyWith(fontWeight: FontWeight.w600),
        minimumSize: const Size(Breakpoints.minTouch, Breakpoints.minTouch),
      ),
    ),
    progressIndicatorTheme: ProgressIndicatorThemeData(
      color: c.primary,
      linearTrackColor: c.track,
      circularTrackColor: c.track,
    ),
    switchTheme: SwitchThemeData(
      thumbColor: WidgetStateProperty.resolveWith((s) => s.contains(WidgetState.selected) ? c.onPrimary : c.outline),
      trackColor: WidgetStateProperty.resolveWith((s) => s.contains(WidgetState.selected) ? c.primary : c.surfaceMuted),
      trackOutlineColor: WidgetStateProperty.all(c.outline),
    ),
    // each platform's own page transition: Material's fade-forwards on Android, the slide (and edge swipe) on iOS
    pageTransitionsTheme: const PageTransitionsTheme(
      builders: {
        TargetPlatform.android: FadeForwardsPageTransitionsBuilder(),
        TargetPlatform.iOS: CupertinoPageTransitionsBuilder(),
        TargetPlatform.linux: FadeForwardsPageTransitionsBuilder(),
        TargetPlatform.macOS: CupertinoPageTransitionsBuilder(),
        TargetPlatform.windows: FadeForwardsPageTransitionsBuilder(),
      },
    ),
  );
}
