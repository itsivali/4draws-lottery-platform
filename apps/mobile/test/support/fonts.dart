import 'dart:io';

import 'package:flutter/services.dart';

bool _loaded = false;

/// Loads the bundled IBM Plex Sans and Material Icons so golden images show real text instead of test boxes.
Future<void> loadAppFonts() async {
  if (_loaded) return;
  _loaded = true;
  final plex = FontLoader('IBMPlexSans');
  for (final w in ['Regular', 'Medium', 'SemiBold', 'Bold']) {
    plex.addFont(_bytes('assets/fonts/IBMPlexSans-$w.ttf'));
  }
  await plex.load();
  final root = Platform.environment['FLUTTER_ROOT'];
  final icons = File('$root/bin/cache/artifacts/material_fonts/MaterialIcons-Regular.otf');
  if (root != null && icons.existsSync()) {
    await (FontLoader('MaterialIcons')..addFont(_bytes(icons.path))).load();
  }
}

Future<ByteData> _bytes(String path) async => ByteData.sublistView(await File(path).readAsBytes());
