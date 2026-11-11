import 'package:flutter/material.dart';

import '../../core/widgets/depth.dart';
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
    // big numbers (redesign 2026-11-11): light and tightly tracked, small muted captions sit over them
    displayLarge: s(60, 64, FontWeight.w300, c.ink, display: true, tracking: -2.4),
    displayMedium: s(48, 54, FontWeight.w300, c.ink, display: true, tracking: -1.8),
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

/// The app's one theme: light, on white (MASTER.md §12).
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
  // pills (redesign 2026-11-11): every button is a stadium
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
    // pages stand on the moving fluid (depth.dart's Grounded, under the router), so scaffolds let it through
    scaffoldBackgroundColor: Colors.transparent,
    canvasColor: c.background,
    dividerColor: c.border,
    focusColor: c.primary.withValues(alpha: 0.16),
    splashFactory: InkSparkle.constantTurbulenceSeedSplashFactory,
    extensions: const [AppTokens(c)],
    visualDensity: VisualDensity.standard,
    materialTapTargetSize: MaterialTapTargetSize.padded,
    appBarTheme: AppBarTheme(
      // clear over the fluid at the top; frosted once the page scrolls under it
      backgroundColor: WidgetStateColor.resolveWith(
        (s) => s.contains(WidgetState.scrolledUnder) ? c.background.withValues(alpha: 0.94) : Colors.transparent,
      ),
      foregroundColor: c.ink,
      surfaceTintColor: Colors.transparent,
      elevation: 0,
      scrolledUnderElevation: 0,
      centerTitle: false,
      titleTextStyle: text.headlineSmall,
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
    filledButtonTheme: FilledButtonThemeData(
      style:
          FilledButton.styleFrom(
            backgroundColor: c.primary,
            foregroundColor: c.onPrimary,
            disabledBackgroundColor: c.surfaceMuted,
            disabledForegroundColor: c.inkMuted,
            minimumSize: buttonSize,
            padding: buttonPadding,
            shape: buttonShape,
            textStyle: buttonText,
            shadowColor: c.accent.withValues(alpha: 0.6),
            // a raised glossy key that sinks when pressed (redesign, second pass)
            backgroundBuilder: (context, states, child) => raisedButtonFace(context, states, child, c: c),
          ).copyWith(
            elevation: WidgetStateProperty.resolveWith(
              (s) => s.contains(WidgetState.disabled)
                  ? 0
                  : s.contains(WidgetState.pressed)
                  ? 1
                  : s.contains(WidgetState.hovered)
                  ? 10
                  : 5,
            ),
          ),
    ),
    outlinedButtonTheme: OutlinedButtonThemeData(
      style: OutlinedButton.styleFrom(
        foregroundColor: c.primary,
        minimumSize: buttonSize,
        padding: buttonPadding,
        shape: buttonShape,
        backgroundColor: c.surface,
        side: BorderSide(color: c.outline),
        textStyle: buttonText,
        elevation: 1,
        shadowColor: c.shadow.withValues(alpha: 0.3),
        backgroundBuilder: (context, states, child) => pearlButtonFace(context, states, child, c: c),
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
    // chips are pills: white with a hairline, or the dark selected segment when chosen (redesign 2026-11-11)
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
    // the bar floats as a white pill; the selected destination is a dark pill with a white icon (redesign 2026-11-11)
    navigationBarTheme: NavigationBarThemeData(
      backgroundColor: Colors.transparent,
      surfaceTintColor: Colors.transparent,
      indicatorColor: c.selected,
      indicatorShape: const StadiumBorder(),
      elevation: 0,
      height: 68,
      labelTextStyle: WidgetStateProperty.resolveWith(
        (s) => s.contains(WidgetState.selected)
            ? text.labelMedium?.copyWith(color: c.ink, fontWeight: FontWeight.w700)
            : text.labelMedium?.copyWith(color: c.inkMuted),
      ),
      iconTheme: WidgetStateProperty.resolveWith(
        (s) => IconThemeData(color: s.contains(WidgetState.selected) ? c.onSelected : c.inkMuted),
      ),
    ),
    navigationRailTheme: NavigationRailThemeData(
      backgroundColor: Colors.transparent,
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
    // a pill toggle: a muted track with the chosen segment dark (redesign 2026-11-11)
    segmentedButtonTheme: SegmentedButtonThemeData(
      style: SegmentedButton.styleFrom(
        backgroundColor: c.surfaceMuted,
        foregroundColor: c.ink,
        selectedBackgroundColor: c.selected,
        selectedForegroundColor: c.onSelected,
        side: BorderSide(color: c.surfaceMuted, width: 3),
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
    pageTransitionsTheme: const PageTransitionsTheme(
      builders: {
        // pages arrive from depth, over the one fluid ground (depth.dart)
        TargetPlatform.android: DepthPageTransitionsBuilder(),
        TargetPlatform.iOS: DepthPageTransitionsBuilder(),
        TargetPlatform.linux: DepthPageTransitionsBuilder(),
        TargetPlatform.macOS: DepthPageTransitionsBuilder(),
        TargetPlatform.windows: DepthPageTransitionsBuilder(),
      },
    ),
  );
}
