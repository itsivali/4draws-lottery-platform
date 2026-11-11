import 'dart:math';

import 'package:flutter/material.dart';
import 'package:flutter/scheduler.dart';

import '../../../../app/theme/tokens.dart';
import '../../application/spin_haptics.dart';
import '../../domain/wheel_geometry.dart';
import '../../domain/wheel_motion.dart';
import 'wheel_fx_painters.dart';
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

  /// Whether the wheel's lights shimmer and chase while it is at rest (they always do while it spins, unless the
  /// platform asks for reduced motion). Tests that wait for the screen to settle turn it off.
  @visibleForTesting
  static bool ambientAnimation = true;

  @override
  State<LotteryWheel> createState() => LotteryWheelState();
}

class LotteryWheelState extends State<LotteryWheel> with TickerProviderStateMixin {
  static final _decorative = WheelGeometry(slots: 12);
  static const _ambientPeriod = Duration(milliseconds: 2600);

  late final Ticker _ticker = createTicker(_onTick);
  final _rotation = ValueNotifier<double>(0);
  final _fx = WheelFx();
  late final AnimationController _ambient = AnimationController(vsync: this, duration: _ambientPeriod)
    ..addListener(() => _fx.update(phase: _ambient.value));
  late final AnimationController _celebrate = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 1600),
  )..addListener(() => _fx.update(celebrate: _celebrate.value));
  int? _landingSlot;

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

  /// The animated layers' state (for tests).
  @visibleForTesting
  WheelFx get fx => _fx;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      if (mounted) _apply();
    });
  }

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    final ambient = LotteryWheel.ambientAnimation && !context.reduceMotion;
    if (ambient && !_ambient.isAnimating) {
      _ambient.repeat();
    } else if (!ambient && _ambient.isAnimating) {
      _ambient.stop();
    }
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
      _landingSlot = s.slot;
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
    _fx.update(
      rotation: r,
      energy: (m.velocityAt(elapsed) / m.timing.peakVelocity).clamp(0.0, 1.0),
      // without the ambient loop (tests) the lights still run with the spin, as a function of its time
      phase: _ambient.isAnimating ? null : (elapsed.inMicroseconds / _ambientPeriod.inMicroseconds) % 1,
    );
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
    _fx.update(energy: 0, rotation: _rotation.value);
    if (hasOutcome && !context.reduceMotion) {
      _fx.update(landedSlot: _landingSlot, clearSlot: _landingSlot == null);
      _celebrate.forward(from: 0);
    }
    if (hasOutcome) widget.onSettled();
  }

  @override
  void dispose() {
    _ticker.dispose();
    _ambient.dispose();
    _celebrate.dispose();
    _rotation.dispose();
    _fx.dispose();
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
            // the wheel stands in 3D: a thick milled rim behind it, a slight backward lean, a wobble at speed
            child: Wheel3D(
              fx: _fx,
              colors: c,
              still: context.reduceMotion,
              child: Stack(
                fit: StackFit.expand,
                children: [
                  CustomPaint(painter: WheelBackplatePainter(c)),
                  ValueListenableBuilder<double>(
                    valueListenable: _rotation,
                    builder: (_, angle, child) => Transform.rotate(angle: angle, child: child),
                    child: Stack(
                      fit: StackFit.expand,
                      children: [
                        RepaintBoundary(
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
                        // the shimmer, twinkle and landing glow turn with the disc
                        if (!context.reduceMotion)
                          RepaintBoundary(
                            child: CustomPaint(
                              painter: WheelDiscFxPainter(fx: _fx, geometry: _geometry, colors: c),
                            ),
                          ),
                      ],
                    ),
                  ),
                  RepaintBoundary(child: CustomPaint(painter: WheelFramePainter(c))),
                  RepaintBoundary(
                    child: CustomPaint(
                      painter: WheelFrameFxPainter(fx: _fx, colors: c),
                    ),
                  ),
                  RepaintBoundary(
                    child: CustomPaint(
                      painter: WheelPointerPainter(fx: _fx, colors: c, geometry: _geometry),
                    ),
                  ),
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
          ),
        );
      },
    );
  }
}

/// The wheel as a solid object: its painted face over a milled gold rim built from thin layers behind it, the whole
/// leaning back a little in perspective so the rim's thickness shows below. It leans toward the pointer or the finger
/// at rest and wobbles slightly with the speed of a spin, as a real wheel on its axle does. Under reduced motion it
/// holds its pose. The pose never changes where the wheel lands: that is the face's rotation alone.
class Wheel3D extends StatefulWidget {
  const Wheel3D({super.key, required this.fx, required this.colors, required this.child, this.still = false});

  final WheelFx fx;
  final AppColors colors;
  final Widget child;

  /// Hold the pose: no lean toward the pointer, no wobble.
  final bool still;

  /// How far back the wheel leans at rest, in radians.
  static const lean = 0.34;

  @override
  State<Wheel3D> createState() => _Wheel3DState();
}

class _Wheel3DState extends State<Wheel3D> {
  Offset _aim = Offset.zero;

  void _point(Offset local) {
    final size = context.size;
    if (size == null || size.isEmpty || widget.still) return;
    setState(
      () => _aim = Offset(
        (local.dx / size.width * 2 - 1).clamp(-1.0, 1.0),
        (local.dy / size.height * 2 - 1).clamp(-1.0, 1.0),
      ),
    );
  }

  void _rest() {
    if (_aim != Offset.zero) setState(() => _aim = Offset.zero);
  }

  static const _layers = 12;

  @override
  Widget build(BuildContext context) {
    final c = widget.colors;
    return MouseRegion(
      onHover: (e) => _point(e.localPosition),
      onExit: (_) => _rest(),
      child: Listener(
        behavior: HitTestBehavior.translucent,
        onPointerDown: (e) => _point(e.localPosition),
        onPointerUp: (_) => _rest(),
        onPointerCancel: (_) => _rest(),
        child: TweenAnimationBuilder<Offset>(
          tween: Tween(end: _aim),
          duration: Motion.emphasis * 1.5,
          curve: _aim == Offset.zero ? Curves.easeOutBack : Curves.easeOutCubic,
          builder: (context, aim, face) => AnimatedBuilder(
            animation: widget.fx,
            child: face,
            builder: (context, face) {
              final e = widget.still ? 0.0 : widget.fx.energy;
              final r = widget.fx.rotation;
              final rx = Wheel3D.lean - aim.dy * 0.14 + sin(r * 2) * 0.03 * e;
              final ry = aim.dx * 0.16 + cos(r * 2) * 0.03 * e;
              return LayoutBuilder(
                builder: (context, box) {
                  final d = box.biggest.shortestSide;
                  final depth = d * 0.11;
                  Matrix4 at(double z) => Matrix4.identity()
                    ..setEntry(3, 2, 0.0009)
                    ..rotateX(rx)
                    ..rotateY(ry)
                    ..translateByDouble(0, 0, z, 1)
                    // cancel the perspective's shrink, so the slices extrude the rim rather than recede inside it
                    ..scaleByDouble(1 + 0.0009 * z, 1 + 0.0009 * z, 1, 1);
                  return Stack(
                    fit: StackFit.expand,
                    children: [
                      // the rim, furthest slice first
                      for (var i = _layers; i >= 1; i--)
                        Transform(
                          alignment: Alignment.center,
                          transform: at(depth * i / _layers),
                          child: CustomPaint(
                            painter: _RimSlice(c, shade: i / _layers, ridge: i.isEven, last: i == _layers),
                          ),
                        ),
                      Transform(alignment: Alignment.center, transform: at(0), child: face),
                    ],
                  );
                },
              );
            },
          ),
          child: widget.child,
        ),
      ),
    );
  }
}

/// One slice of the wheel's rim: gold darkening with depth, alternately milled; the last carries a soft shadow.
class _RimSlice extends CustomPainter {
  _RimSlice(this.c, {required this.shade, required this.ridge, required this.last});

  final AppColors c;
  final double shade;
  final bool ridge;
  final bool last;

  @override
  void paint(Canvas canvas, Size size) {
    final o = size.center(Offset.zero);
    final r = size.shortestSide / 2;
    if (last) {
      canvas.drawCircle(
        o,
        r * 1.02,
        Paint()
          ..color = c.shadow.withValues(alpha: 0.35)
          ..maskFilter = const MaskFilter.blur(BlurStyle.normal, 14),
      );
    }
    // bronze, darkening with depth, so the thickness reads against the bright frame
    final base = Color.lerp(c.wheelRimDark, Colors.black, 0.15 + 0.35 * shade)!;
    canvas.drawCircle(o, r * 0.995, Paint()..color = ridge ? base : Color.lerp(base, Colors.black, 0.12)!);
  }

  @override
  bool shouldRepaint(_RimSlice old) => old.shade != shade || old.ridge != ridge || old.c != c;
}
