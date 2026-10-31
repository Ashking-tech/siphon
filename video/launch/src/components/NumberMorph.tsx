import type {CSSProperties} from 'react';
import {useCurrentFrame} from 'remotion';
import {EASE, range} from '../lib/anim';

export type NumberMorphProps = {
  from: number;
  to: number;
  start?: number;
  duration?: number;
  pad?: number;
  prefix?: string;
  suffix?: string;
  style?: CSSProperties;
  digitStyle?: CSSProperties;
};

const DIGITS = ['0', '1', '2', '3', '4', '5', '6', '7', '8', '9'];

const digitsOf = (value: number, pad: number) => Math.abs(Math.trunc(value)).toString().padStart(pad, '0').split('');

/**
 * A mechanical odometer. Each digit position rolls to its new value on its own
 * column; used for technical counters, never for headline numerals.
 */
export const NumberMorph: React.FC<NumberMorphProps> = ({
  from,
  to,
  start = 0,
  duration = 30,
  pad = 2,
  prefix,
  suffix,
  style,
  digitStyle,
}) => {
  const frame = useCurrentFrame();
  const p = range(frame, start, start + duration, 0, 1, EASE.inOut);
  const fromDigits = digitsOf(from, pad);
  const toDigits = digitsOf(to, pad);
  const length = Math.max(fromDigits.length, toDigits.length);

  return (
    <div
      style={{
        display: 'inline-flex',
        alignItems: 'baseline',
        fontVariantNumeric: 'tabular-nums',
        ...style,
      }}
    >
      {prefix ? <span style={{whiteSpace: 'pre'}}>{prefix}</span> : null}
      {Array.from({length}, (_, i) => {
        const fromDigit = Number(fromDigits[fromDigits.length - length + i] ?? 0);
        const toDigit = Number(toDigits[toDigits.length - length + i] ?? 0);
        const increasing = toDigit >= fromDigit;
        const travel = increasing ? toDigit - fromDigit : fromDigit - toDigit;
        const roll = increasing ? fromDigit + travel * p : fromDigit - travel * p;

        return (
          <span
            key={i}
            style={{
              display: 'inline-block',
              overflow: 'hidden',
              height: '1em',
              lineHeight: 1,
              verticalAlign: 'baseline',
              ...digitStyle,
            }}
          >
            <span
              style={{
                display: 'block',
                transform: `translateY(${-roll}em)`,
                willChange: 'transform',
              }}
            >
              {DIGITS.map((digit) => (
                <span key={digit} style={{display: 'block', height: '1em'}}>
                  {digit}
                </span>
              ))}
            </span>
          </span>
        );
      })}
      {suffix ? <span style={{whiteSpace: 'pre'}}>{suffix}</span> : null}
    </div>
  );
};
