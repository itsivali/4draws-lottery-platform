import 'dart:io';

import 'package:flutter_test/flutter_test.dart';

/// S8: the public policy pages the stores ask for are plain web pages in the web build (readable without the app or
/// JavaScript), complete (no open items), in the operator's legal name, honest that the service is not yet licensed,
/// linked to each other and listed in the sitemap.
void main() {
  const slugs = ['privacy', 'terms', 'responsible-gambling', 'delete-account'];
  final sitemap = File('web/sitemap.xml').readAsStringSync();

  for (final slug in slugs) {
    test('/$slug/ is a complete page with its canonical address, linked from the others', () {
      final html = File('web/$slug/index.html').readAsStringSync();
      expect(html, contains('<link rel="canonical" href="https://tajirika.co.ke/$slug/">'));
      expect(html, contains('class="notice"'));
      expect(html, contains('not yet licensed'), reason: 'the service status is stated on every page');
      expect(html, contains('4 DRAWS TICKET LTD'), reason: "the operator's legal name");
      expect(html, isNot(contains('TO CONFIRM')), reason: 'no open items');
      expect(html, contains('Effective 11 November 2026'));
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

  test('deletion follows the usual shape: what to do first, the alternative, the steps, what goes and what stays', () {
    final html = File('web/delete-account/index.html').readAsStringSync();
    for (final part in [
      'Before you delete your account',
      'Take a break instead',
      'How to delete your account',
      'What happens next',
      'What is deleted',
      'What we must keep',
      'Changed your mind?',
    ]) {
      expect(html, contains(part), reason: part);
    }
  });

  test('the privacy statement names the rights, the regulator of data and how to complain', () {
    final html = File('web/privacy/index.html').readAsStringSync();
    expect(html, contains('Data Protection Act, 2019'));
    expect(html, contains('Office of the Data Protection'));
    expect(html, contains('privacy@4draws.co.ke'));
  });

  test('the terms cover eligibility, money, disputes and the governing law', () {
    final html = File('web/terms/index.html').readAsStringSync();
    for (final part in ['18 years or older', 'The wheel never pays cash', 'Complaints and disputes', 'laws of Kenya']) {
      expect(html, contains(part), reason: part);
    }
  });
}
