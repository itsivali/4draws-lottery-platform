import 'package:flutter/services.dart';

/// The feedback moments the app uses (design-system MASTER.md §9). Widgets never call [HapticFeedback] directly.
enum HapticEvent {
  /// A play was accepted locally and the wheel starts.
  start,

  /// The pointer crossed a segment boundary while the wheel slows.
  tick,

  /// The wheel came to rest on the server's slot.
  stop,

  /// The server confirmed a money or state change (credited, paid).
  confirmed,
}

abstract interface class Haptics {
  void play(HapticEvent event);
}

/// No device feedback: the web, tests, or devices without a motor.
class SilentHaptics implements Haptics {
  const SilentHaptics();

  @override
  void play(HapticEvent event) {}
}

/// The platform's own feedback primitives (iOS UIFeedbackGenerator, Android HapticFeedbackConstants). The operating
/// system decides how, or whether, they are felt; nothing here assumes they always are.
class PlatformHaptics implements Haptics {
  const PlatformHaptics();

  @override
  void play(HapticEvent event) {
    final Future<void> f = switch (event) {
      HapticEvent.start => HapticFeedback.lightImpact(),
      HapticEvent.tick => HapticFeedback.selectionClick(),
      HapticEvent.stop => HapticFeedback.mediumImpact(),
      HapticEvent.confirmed => HapticFeedback.lightImpact(),
    };
    f.ignore(); // feedback is best effort; a platform that cannot vibrate must never break a play
  }
}

/// Feedback only while the player's *Wheel haptics* setting is on. The setting never touches an outcome.
class GatedHaptics implements Haptics {
  const GatedHaptics(this.inner, {required this.enabled});

  final Haptics inner;
  final bool Function() enabled;

  @override
  void play(HapticEvent event) {
    if (enabled()) inner.play(event);
  }
}

Haptics hapticsForPlatform({required bool isWeb, required bool Function() enabled}) =>
    isWeb ? const SilentHaptics() : GatedHaptics(const PlatformHaptics(), enabled: enabled);
