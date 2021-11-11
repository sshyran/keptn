import { Component, ElementRef, OnInit, ViewChild } from '@angular/core';
import { Chart, ChartConfiguration, registerables } from 'chart.js';
import 'chartjs-chart-matrix';
import { ResultTypes } from '../../../../shared/models/result-types';
// import { MatrixDataPoint } from 'chartjs-chart-matrix';
import { MatrixController, MatrixElement } from 'chartjs-chart-matrix';

Chart.register(MatrixController, MatrixElement, ...registerables);

// MatrixDataPoint is just number but it can also be string
type MatrixPoint = { x: string; y: string; v: number };

const resultMapping: { [key: string]: number } = {
  [ResultTypes.FAILED]: 0,
  [ResultTypes.WARNING]: 1,
  [ResultTypes.PASSED]: 2,
};

@Component({
  selector: 'ktb-chart-matrix',
  templateUrl: './ktb-chart-matrix.component.html',
  styleUrls: ['./ktb-chart-matrix.component.scss'],
})
export class KtbChartMatrixComponent implements OnInit {
  @ViewChild('myChart', { static: true }) myChart?: ElementRef<HTMLCanvasElement>;
  private xCategories: string[] = [];
  private yCategories: string[] = [];

  public ngOnInit(): void {
    const ctx = this.myChart?.nativeElement.getContext('2d');
    if (ctx) {
      const data = this.generateData(20);
      const config: ChartConfiguration<'matrix'> = {
        type: 'matrix',
        data: {
          datasets: [
            {
              // @ts-ignore
              data,
              backgroundColor(context): string {
                // @ts-ignore
                const value = context.dataset.data[context.dataIndex].v;
                let color: string;
                if (value === 0) {
                  color = '#dc172a';
                } else if (value === 1) {
                  color = '#e6be00';
                } else {
                  color = '#7dc540';
                }
                return color;
                // const alpha = (value - 5) / 40;
                // return helpers.color('green').alpha(alpha).rgbString();
              },
              // borderColor(context): string {
              //   // @ts-ignore
              //   const value = context.dataset.data[context.dataIndex].v;
              //   const alpha = (value - 5) / 40;
              //   return helpers.color('darkgreen').alpha(alpha).rgbString();
              // },
              borderWidth: 1,
              width: ({ chart }: { chart: Chart }): number => 100,
              height: ({ chart }: { chart: Chart }): number =>
                (chart.chartArea || {}).height / this.yCategories.length - 1,
            },
          ],
        },
        options: {
          scales: {
            // @ts-ignore
            x: {
              type: 'category',
              labels: this.xCategories,
              ticks: {
                display: true,
              },
              grid: {
                display: false,
              },
            },
            // @ts-ignore
            y: {
              type: 'category',
              labels: ['score', 'response time p95'],
              offset: true,
              ticks: {
                display: true,
              },
              grid: {
                display: false,
              },
            },
          },

          plugins: {
            // @ts-ignore
            legend: false,
            tooltip: {
              callbacks: {
                // @ts-ignore
                title(): string {
                  return '';
                },
                // @ts-ignore
                label(context): string[] {
                  const v = context.dataset.data[context.dataIndex];
                  // @ts-ignore
                  return ['x: ' + v.x, 'y: ' + v.y, 'v: ' + v.v];
                },
              },
            },
          },
        },
        plugins: [],
      };
      const _chartElement = new Chart(ctx, config);
      // chartElement.render();
    }
  }

  private generateData(counter: number): MatrixPoint[] {
    this.yCategories = ['score', 'response time p95'];
    const points = [];
    for (const category of this.yCategories.reverse()) {
      points.push(...this.generateDataFor(category, counter));
    }

    const categories = points.map((p) => p.x);
    this.xCategories = categories.filter((p, i) => categories.indexOf(p) === i);
    return points;
  }

  private generateDataFor(category: string, counter: number): MatrixPoint[] {
    const points = [];
    for (let i = 1; i <= counter; ++i) {
      points.push({
        x: `Date_${i}`,
        y: category,
        v: Math.floor(Math.random() * 3),
      });
    }
    return points;
  }
}
