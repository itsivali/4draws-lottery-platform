import 'package:flutter/material.dart';
import 'package:flutter/services.dart';

import '../../app/theme/tokens.dart';
import 'brand_mark.dart';
import 'reveal.dart';

/// The signed-out page (owner, 2026-11-11): the brand's indigo fills the whole screen and the form sits in the middle
/// of it, on one white card with a deep shadow. Sign-in, create account and the password-reset screens share it, on
/// phones and on wide screens alike. The card carries its own heading; the mark stands above it in white.
class AuthPage extends StatelessWidget {
  const AuthPage({super.key, required this.title, required this.lead, required this.child, this.footer});

  final String title;
  final String lead;

  /// The form.
  final Widget child;

  /// Under the card, on the colour (a quiet link, a line of small print).
  final Widget? footer;

  static const _cardWidth = 440.0;

  @override
  Widget build(BuildContext context) {
    final c = context.colors;
    final t = Theme.of(context).textTheme;
    return AnnotatedRegion<SystemUiOverlayStyle>(
      // white status-bar icons over the indigo
      value: SystemUiOverlayStyle.light,
      child: Scaffold(
        backgroundColor: c.heroEnd,
        body: Stack(
          fit: StackFit.expand,
          children: [
            ExcludeSemantics(
              child: DecoratedBox(
                decoration: BoxDecoration(gradient: Gradients.hero(c)),
                child: DecoratedBox(
                  decoration: BoxDecoration(gradient: Gradients.heroGlow(c)),
                  child: DecoratedBox(
                    // a faint gold light from the far corner, the logo's fourth segment
                    decoration: BoxDecoration(
                      gradient: Gradients.blob(
                        c.wheelRimLight,
                        center: const Alignment(1.1, 1.15),
                        radius: 0.9,
                        alpha: 0.16,
                      ),
                    ),
                  ),
                ),
              ),
            ),
            SafeArea(
              child: Center(
                child: SingleChildScrollView(
                  padding: const EdgeInsets.symmetric(horizontal: Space.lg, vertical: Space.xl),
                  child: ConstrainedBox(
                    constraints: const BoxConstraints(maxWidth: _cardWidth),
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                        Center(child: BrandMark(height: 40, wordmarkColor: c.onHero)),
                        const SizedBox(height: Space.xl),
                        Reveal(
                          child: DecoratedBox(
                            decoration: BoxDecoration(
                              borderRadius: BorderRadius.circular(Radii.xl),
                              boxShadow: Elevation.sheet(c),
                            ),
                            child: Material(
                              color: c.surface,
                              borderRadius: BorderRadius.circular(Radii.xl),
                              clipBehavior: Clip.antiAlias,
                              child: Padding(
                                padding: const EdgeInsets.all(Space.xl),
                                // the card grows and shrinks smoothly as the form changes (sign in ↔ create account)
                                child: AnimatedSize(
                                  duration: context.reduceMotion ? Duration.zero : Motion.emphasis,
                                  curve: Motion.settle,
                                  alignment: Alignment.topCenter,
                                  child: Column(
                                    crossAxisAlignment: CrossAxisAlignment.stretch,
                                    children: [
                                      Semantics(header: true, child: Text(title, style: t.headlineMedium)),
                                      const SizedBox(height: Space.xs),
                                      Text(lead, style: t.bodyMedium?.copyWith(color: c.inkMuted)),
                                      const SizedBox(height: Space.xl),
                                      child,
                                    ],
                                  ),
                                ),
                              ),
                            ),
                          ),
                        ),
                        if (footer != null) ...[const SizedBox(height: Space.lg), footer!],
                      ],
                    ),
                  ),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}
