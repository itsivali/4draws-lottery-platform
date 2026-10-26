import 'dart:math';

import 'package:flutter/material.dart';
import 'package:flutter/scheduler.dart';

import '../../../../app/theme/tokens.dart';
import '../../application/spin_haptics.dart';
import '../../domain/wheel_geometry.dart';
import '../../domain/wheel_motion.dart';
import 'wheel_painters.dart';

/// What the wheel should be doing, derived from the play's state. The wheel never chooses anything: it spins without
/// meaning while [spinId] is set and no [slot] is known, slows onto [slot] once the server gave it, and comes to rest
/// without an outcome when [abort] is set.
@immutable
class WheelSpin {
  const WheelSpin({this.spinId, this.slot, this.geometry, this.within = 0.5, this.abort = false});

  const WheelSpin.idle() : this();

  /// The play's idempotency key while a play is in hand.
  final String? spinId;

  /// The server's slot.
  final int? slot;

  /// The server's wheel (slot count and lucky slots) for this spin.
  final WheelGeometry? geometry;

  /// Where inside the slot to stop (see [landingFraction]).
  final double within;

  /// No outcome will come: the play was refused or its answer was lost.
  final bool abort;
}

class LotteryWheel extends StatefulWidget {
  const LotteryWheel({
    super.key,
    required this.spin,
    required this.restingGeometry,
    required this.haptics,
    required this.onSettled,
    this.maxDiameter = 460,
    this.timing = const SpinTiming(),
  });

  final WheelSpin spin;

  /// The wheel to show at rest: the server's from today's plays, or null when not known yet (then the segments are
  /// plain decoration, with no numbers and no lucky marks).
  final WheelGeometry? restingGeometry;
  final SpinHaptics haptics;

  /// Called once per spin when the wheel has come to rest on the server's slot.
  final VoidCallback onSettled;
  final double maxDiameter;
  final SpinTiming timing;

  @override
  State<LotteryWheel> createState() => LotteryWheelState();
}

class LotteryWheelState extends State<LotteryWheel> with SingleTickerProviderStateMixin {
  static final _decorative = WheelGeometry(slots: 12);

  late final Ticker _ticker = createTicker(_onTick);
  final _rotation = ValueNotifier<double>(0);

  WheelMotion? _motion;
  String? _activeSpin;
  String? _landedSpin;
  String? _settledSpin;
  WheelGeometry? _spinGeometry;
  Duration _elapsed = Duration.zero;

  /// The rotation the wheel is drawn at (for tests).
  @visibleForTesting
  double get rotation => _rotation.value;

  /// The wheel being drawn (for tests).
  @visibleForTesting
  WheelGeometry get geometry => _geometry;

  WheelGeometry get _geometry => _spinGeometry ?? widget.restingGeometry ?? _decorative;
  bool get _labelled => _spinGeometry != null || widget.restingGeometry != null;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) _apply();
    });
  }

  @override
  void didUpdateWidget(LotteryWheel oldWidget) {
    super.didUpdateWidget(oldWidget);
    _apply();
  }

  void _apply() {
    final s = widget.spin;
    final reduce = context.reduceMotion;
    if (s.spinId == null) {
      if (_activeSpin != null && _ticker.isActive && _motion?.target == null) _motion?.stop(at: _elapsed);
      if (_activeSpin != null && !_ticker.isActive) {
        setState(() {
          _activeSpin = null;
          _spinGeometry = null;
        });
      }
      return;
    }
    if (s.spinId != _activeSpin) {
      _activeSpin = s.spinId;
      _landedSpin = null;
      _settledSpin = null;
      _motion = null;
      // a play found again after its answer was lost arrives already aborted: nothing spins until its slot is known
      if (!s.abort) {
        widget.haptics.begin(s.spinId!);
        if (!reduce) _startMotion();
      }
    }
    if (s.slot != null && _landedSpin != s.spinId) {
      _landedSpin = s.spinId;
      final g = s.geometry ?? _geometry;
      if (g != _spinGeometry) setState(() => _spinGeometry = g);
      if (reduce) {
        // no spin: settle straight onto the server's slot
        _ticker.stop();
        _motion = null;
        _rotation.value = g.targetRotation(from: _rotation.value, segment: s.slot!, within: s.within);
        SchedulerBinding.instance.addPostFrameCallback((_) => _settle(hasOutcome: true));
        return;
      }
      if (_motion == null || _motion!.target != null || !_ticker.isActive) {
        // the wheel had stopped (a recovered play): give the confirmed result its own short spin
        _settledSpin = null;
        widget.haptics.begin('${s.spinId!}:recovered');
        _startMotion();
      }
      _motion!.land(at: _elapsed, geometry: g, slot: s.slot!, within: s.within);
    } else if (s.abort && _motion != null && _motion!.target == null) {
      _motion!.stop(at: _elapsed);
    }
  }

  void _startMotion() {
    _ticker.stop();
    _elapsed = Duration.zero;
    _motion = WheelMotion.start(rotation: normalizeAngle(_rotation.value), at: Duration.zero, timing: widget.timing);
    _ticker.start();
  }

  void _onTick(Duration elapsed) {
    final m = _motion;
    if (m == null) return;
    _elapsed = elapsed;
    final r = m.rotationAt(elapsed);
    _rotation.value = r;
    widget.haptics.onFrame(
      at: elapsed,
      geometry: _geometry,
      rotation: r,
      velocity: m.velocityAt(elapsed),
      phase: m.phaseAt(elapsed),
    );
    if (m.settledAt(elapsed)) {
      _ticker.stop();
      _settle(hasOutcome: m.hasOutcome);
    }
  }

  void _settle({required bool hasOutcome}) {
    if (!mounted || _settledSpin == _activeSpin) return;
    _settledSpin = _activeSpin;
    widget.haptics.onSettled(hasOutcome: hasOutcome);
    if (hasOutcome) widget.onSettled();
  }

  @override
  void dispose() {
    _ticker.dispose();
    _rotation.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    final scaler = MediaQuery.textScalerOf(context);
    final waiting = widget.spin.spinId != null && widget.spin.slot == null && !widget.spin.abort;
    return LayoutBuilder(
      builder: (context, box) {
        final d = [
          box.maxWidth,
          box.maxHeight.isFinite ? box.maxHeight : double.infinity,
          widget.maxDiameter,
        ].reduce(min);
        return ExcludeSemantics(
          // decorative: the result panel says what happened, in words
          child: SizedBox.square(
            dimension: d,
            child: Stack(
              fit: StackFit.expand,
              children: [
                CustomPaint(painter: WheelBackplatePainter(c)),
                ValueListenableBuilder<double>(
                  valueListenable: _rotation,
                  builder: (_, angle, child) => Transform.rotate(angle: angle, child: child),
                  child: RepaintBoundary(
                    child: AnimatedSwitcher(
                      duration: Motion.standard,
                      child: SizedBox.expand(
                        key: ValueKey((_geometry.slots, _geometry.luckySlots, _labelled)),
                        child: CustomPaint(
                          painter: WheelDiscPainter(
                            geometry: _geometry,
                            colors: c,
                            labelled: _labelled,
                            textScaler: scaler,
                          ),
                        ),
                      ),
                    ),
                  ),
                ),
                RepaintBoundary(child: CustomPaint(painter: WheelFramePainter(c))),
                if (waiting && context.reduceMotion)
                  Center(
                    child: SizedBox.square(
                      dimension: d * 0.12,
                      child: CircularProgressIndicator(strokeWidth: 3, color: c.wheelRimLight),
                    ),
                  ),
              ],
            ),
          ),
        );
      },
    );
  }
}
