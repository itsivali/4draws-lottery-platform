import 'dart:math';

import 'wheel_geometry.dart';

/// The shape of a spin. Tuned on devices; change here, nowhere else. Quicker since 2026-10-30 (owner): about two and
/// a half seconds from the tap to rest when the answer is quick, at a higher top speed.
class SpinTiming {
  const SpinTiming({
    this.accelerate = const Duration(milliseconds: 350),
    this.minCruise = const Duration(milliseconds: 500),
    this.decelerate = const Duration(milliseconds: 1800),
    this.abort = const Duration(milliseconds: 700),
    this.peakTurnsPerSecond = 3.0,
  });

  /// From rest to cruising speed (velocity follows a smoothstep, so there is no jolt at either end).
  final Duration accelerate;

  /// The least time at cruising speed before slowing starts, even when the answer is already in.
  final Duration minCruise;

  /// The preferred slowing time; the real one varies by up to a quarter so the stop lands on the slot exactly.
  final Duration decelerate;

  /// The slowing time when the spin ends without an answer.
  final Duration abort;

  final double peakTurnsPerSecond;

  double get peakVelocity => peakTurnsPerSecond * 2 * pi;
}

enum MotionPhase { accelerating, cruising, decelerating, settled }

double _s(Duration d) => d.inMicroseconds / 1e6;

/// The wheel's rotation as a pure function of time since the spin began. Nothing here is random and nothing reads a
/// frame counter: the outcome-bound stop is planned once, from the server's slot, when [land] is called, and every
/// frame after that only evaluates the plan. Rebuilds, resizes or dropped frames cannot change where it stops.
///
/// Phases: accelerate (smoothstep velocity), cruise (constant, for as long as the answer takes), decelerate (an
/// ease-out cubic whose start velocity equals the cruising speed, so the hand-off is seamless), settled.
class WheelMotion {
  WheelMotion.start({required double rotation, required Duration at, this.timing = const SpinTiming()})
    : _startRotation = rotation,
      _startedAt = at;

  final SpinTiming timing;
  final double _startRotation;
  final Duration _startedAt;

  Duration? _decelAt;
  double? _decelFrom;
  double? _travel;
  Duration? _decelFor;
  double? _target;
  bool _hasOutcome = false;

  /// The final rotation, once an answer has been planned.
  double? get target => _target;

  /// Whether the stop shows a server outcome (false for a spin stopped without an answer).
  bool get hasOutcome => _hasOutcome;

  /// When the wheel comes to rest, once slowing is planned.
  Duration? get settlesAt => _decelAt == null ? null : _decelAt! + _decelFor!;

  /// Plans the stop on [slot] of [geometry], [within] of the way through it. Only the first call counts.
  void land({required Duration at, required WheelGeometry geometry, required int slot, required double within}) {
    if (_decelAt != null) return;
    final begin = _laterOf(at, _startedAt + timing.accelerate + timing.minCruise);
    final from = rotationAt(begin);
    final preferred = timing.peakVelocity * _s(timing.decelerate) / 3; // ease-out cubic: D = 3Δ/ω
    final target = geometry.targetNear(
      from: from,
      segment: slot,
      preferredTravel: preferred,
      minTravel: 0.7 * preferred,
      within: within,
    );
    _plan(begin, from, target - from);
    _target = target;
    _hasOutcome = true;
  }

  /// Slows to rest wherever the wheel is, because no answer will come. Ignored once a stop is planned.
  void stop({required Duration at}) {
    if (_decelAt != null) return;
    final begin = _laterOf(at, _startedAt);
    final v = velocityAt(begin);
    final from = rotationAt(begin);
    _plan(begin, from, v * _s(timing.abort) / 3);
  }

  void _plan(Duration begin, double from, double travel) {
    final v = velocityAt(begin);
    _decelAt = begin;
    _decelFrom = from;
    _travel = travel;
    _decelFor = v <= 0 || travel <= 0 ? Duration.zero : Duration(microseconds: (3 * travel / v * 1e6).round());
  }

  MotionPhase phaseAt(Duration t) {
    final e = t - _startedAt;
    if (_decelAt != null && t >= _decelAt!) {
      return t >= settlesAt! ? MotionPhase.settled : MotionPhase.decelerating;
    }
    return e < timing.accelerate ? MotionPhase.accelerating : MotionPhase.cruising;
  }

  bool settledAt(Duration t) => phaseAt(t) == MotionPhase.settled;

  double rotationAt(Duration t) {
    if (_decelAt != null && t >= _decelAt!) {
      if (_decelFor == Duration.zero) return _decelFrom! + _travel!;
      final u = min(1.0, _s(t - _decelAt!) / _s(_decelFor!));
      final rest = 1 - u;
      return _decelFrom! + _travel! * (1 - rest * rest * rest);
    }
    final e = max(0.0, _s(t - _startedAt));
    final ta = _s(timing.accelerate);
    final w = timing.peakVelocity;
    if (e < ta) {
      final u = e / ta;
      return _startRotation + w * ta * (u * u * u - u * u * u * u / 2); // ∫ w·(3u²−2u³)
    }
    return _startRotation + w * ta / 2 + w * (e - ta);
  }

  double velocityAt(Duration t) {
    if (_decelAt != null && t >= _decelAt!) {
      if (_decelFor == Duration.zero) return 0;
      final u = min(1.0, _s(t - _decelAt!) / _s(_decelFor!));
      final rest = 1 - u;
      return 3 * _travel! / _s(_decelFor!) * rest * rest;
    }
    final e = max(0.0, _s(t - _startedAt));
    final ta = _s(timing.accelerate);
    if (e < ta) {
      final u = e / ta;
      return timing.peakVelocity * (3 * u * u - 2 * u * u * u);
    }
    return timing.peakVelocity;
  }

  static Duration _laterOf(Duration a, Duration b) => a > b ? a : b;
}
