// Package training はトレーニングの進行を司るドメイン層である。
//
// この層は Onion Architecture の最内周にあたり、外側（Application、
// Infrastructure、Presentation）を一切知らない。標準ライブラリ以外への
// 依存を持たないため、DB も HTTP も立てずに全機能をテストできる。
package training
