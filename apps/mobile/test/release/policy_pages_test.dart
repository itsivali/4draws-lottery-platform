import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

/// S8: the public policy pages the stores ask for are plain web pages in the web build (readable without the app or
/// JavaScript), marked as drafts until counsel signs them off, linked to each other and listed in the sitemap.
void main() {
  const slugs = ['privacy', 'terms', 'responsible-gambling', 'delete-account'];
  final sitemap = File('web/sitemap.xml').readAsStringSync();

  for (final slug in slugs) {
    test('/$slug/ is a draft page with its canonical address, linked from the others', () {
      final html = File('web/$slug/index.html').readAsStringSync();
      expect(html, contains('<link rel="canonical" href="https://tajirika.co.ke/$slug/">'));
      expect(html, contains('class="draft"'));
      expect(html, contains('not yet licensed'));
      expect(html, isNot(contains('<script')), reason: 'readable without JavaScript');
      for (final other in slugs) {
        expect(html, contains('href="/$other/"'), reason: 'links to /$other/');
      }
      expect(sitemap, contains('<loc>https://tajirika.co.ke/$slug/</loc>'));
    });
  }

  test('deleting an account can be asked for without the app', () {
    final html = File('web/delete-account/index.html').readAsStringSync();
    expect(html, contains('Without signing in'));
    expect(html, contains('What we must keep'));
  });
}
