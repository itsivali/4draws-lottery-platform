import 'dart:math';

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/app/theme/tokens.dart';

double _channel(double c) => c <= 0.03928 ? c / 12.92 : pow((c + 0.055) / 1.055, 2.4).toDouble();

double luminance(Color c) => 0.2126 * _channel(c.r) + 0.7152 * _channel(c.g) + 0.0722 * _channel(c.b);

double contrast(Color a, Color b) {
  final la = luminance(a), lb = luminance(b);
  return (max(la, lb) + 0.05) / (min(la, lb) + 0.05);
}

void main() {
  for (final (name, c) in [('light', AppColors.light)]) {
    group('$name theme (WCAG 2.2 AA)', () {
      final surfaces = {'background': c.background, 'surface': c.surface, 'surfaceMuted': c.surfaceMuted};
      final texts = {
        'ink': c.ink,
        'inkMuted': c.inkMuted,
        'primary': c.primary,
        'accent': c.accent,
        'goldText': c.goldText,
        'success': c.success,
        'warning': c.warning,
        'error': c.error,
        'pending': c.pending,
      };
      for (final t in texts.entries) {
        for (final s in surfaces.entries) {
          test('${t.key} text on ${s.key} is at least 4.5:1', () {
            expect(contrast(t.value, s.value), greaterThanOrEqualTo(4.5));
          });
        }
      }
      test('text on primary and on primary containers is at least 4.5:1', () {
        expect(contrast(c.onPrimary, c.primary), greaterThanOrEqualTo(4.5));
        expect(contrast(c.onPrimaryContainer, c.primaryContainer), greaterThanOrEqualTo(4.5));
      });
      test('control outlines and the focus ring are at least 3:1 against the page', () {
        expect(contrast(c.outline, c.background), greaterThanOrEqualTo(3));
        expect(contrast(c.outline, c.surface), greaterThanOrEqualTo(3));
        expect(contrast(c.primary, c.background), greaterThanOrEqualTo(3));
      });
      test('wheel labels are readable on every segment colour', () {
        expect(c.wheelSegments.length, greaterThanOrEqualTo(2));
        for (final seg in [...c.wheelSegments, c.wheelLucky]) {
          expect(contrast(c.wheelLabel, seg), greaterThanOrEqualTo(4.5));
        }
        expect(c.wheelSegments.toSet().length, c.wheelSegments.length, reason: 'each hue is used once per turn');
        expect(
          contrast(c.wheelStar, c.wheelLucky),
          greaterThanOrEqualTo(3),
          reason: 'the star is a symbol on the lucky slot',
        );
        for (final seg in c.wheelSegments) {
          expect(
            contrast(c.wheelChecker, seg),
            greaterThanOrEqualTo(1.4),
            reason: 'the checker must be visible on every hue',
          );
        }
      });
      test('the pointer and rim stand out from the page (non-text, 3:1)', () {
        expect(contrast(c.gold, c.background), greaterThanOrEqualTo(3));
      });
    });
  }

  test('pending never borrows the success colour, and gold is not the primary', () {
    expect(AppColors.light.pending, isNot(AppColors.light.success));
    expect(AppColors.light.gold, isNot(AppColors.light.primary));
  });
  test('the hero surfaces keep their text readable at both ends of the sunset', () {
    final c = AppColors.light;
    expect(contrast(c.onHero, c.heroStart), greaterThanOrEqualTo(4.5));
    expect(contrast(c.onHero, c.heroEnd), greaterThanOrEqualTo(4.5));
    expect(contrast(c.onHeroMuted, c.heroStart), greaterThanOrEqualTo(3));
  });
}
