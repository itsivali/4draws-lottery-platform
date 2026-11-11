import 'package:flutter/material.dart';

import '../../app/theme/tokens.dart';

/// An icon in a round tonal container (Material 3): the tone's colour as the glyph over a light wash of it. It heads
/// cards, rows, notices and empty states. Decorative: the words beside it say what it means.
class IconDisc extends StatelessWidget {
  const IconDisc({super.key, required this.icon, this.size = 40, this.color, this.filled = false});

  final IconData icon;
  final double size;

  /// The tone: the primary by default, a status colour on a notice, a row's own colour.
  final Color? color;

  /// A solid disc with a white glyph, for the one place that should lead the eye.
  final bool filled;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    final tone = color ?? c.primary;
    return ExcludeSemantics(
      child: Container(
        width: size,
        height: size,
        decoration: BoxDecoration(
          shape: BoxShape.circle,
          color: filled ? tone : (color == null ? c.primaryContainer : tone.withValues(alpha: 0.12)),
        ),
        child: Icon(
          icon,
          size: size * 0.5,
          color: filled ? c.onPrimary : (color == null ? c.onPrimaryContainer : tone),
        ),
      ),
    );
  }
}
