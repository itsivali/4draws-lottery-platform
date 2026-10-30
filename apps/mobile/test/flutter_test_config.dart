import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:lottery_mobile/features/play/presentation/widgets/lottery_wheel.dart';

/// Golden images may differ by a few antialiased pixels between machines with the same Flutter; real regressions
/// change far more than this.
const _tolerance = 0.005;

Future<void> testExecutable(FutureOr<void> Function() testMain) async {
  // the wheel's idle shimmer never ends, so screens holding it would never settle (its own test turns it back on)
  LotteryWheel.ambientAnimation = false;
  if (goldenFileComparator is LocalFileComparator) {
    goldenFileComparator = _TolerantComparator((goldenFileComparator as LocalFileComparator).basedir.resolve('x.dart'));
  }
  await testMain();
}

class _TolerantComparator extends LocalFileComparator {
  _TolerantComparator(super.testFile);

  @override
  Future<bool> compare(Uint8List imageBytes, Uri golden) async {
    final result = await GoldenFileComparator.compareLists(imageBytes, await getGoldenBytes(golden));
    if (result.passed || result.diffPercent <= _tolerance) {
      result.dispose();
      return true;
    }
    final error = await generateFailureOutput(result, golden, basedir);
    result.dispose();
    throw FlutterError(error);
  }
}
