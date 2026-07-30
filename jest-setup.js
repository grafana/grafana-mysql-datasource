import nodeCrypto from 'crypto';
import React from 'react';
import './.config/jest-setup';
import { matchers } from './src/test/matchers';

global.React = React;

// jsdom's `crypto` doesn't implement `randomUUID` (added upstream after the
// jsdom version this Jest environment bundles), so polyfill it with Node's.
if (typeof global.crypto.randomUUID !== 'function') {
  global.crypto.randomUUID = nodeCrypto.randomUUID.bind(nodeCrypto);
}

const mockIntersectionObserver = jest.fn().mockImplementation((callback) => ({
  observe: jest.fn().mockImplementation((elem) => {
    callback([{ target: elem, isIntersecting: true }]);
  }),
  unobserve: jest.fn(),
  disconnect: jest.fn(),
}));
global.IntersectionObserver = mockIntersectionObserver;

expect.extend(matchers);
