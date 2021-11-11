import { Component } from '@angular/core';
import {
  ApexAnnotations,
  ApexAxisChartSeries,
  ApexChart,
  ApexDataLabels,
  ApexPlotOptions,
  ApexTooltip,
} from 'ng-apexcharts';
import { Evaluations } from '../../_services/_mockData/evaluations.mock';
import { Trace } from '../../_models/trace';
import { ResultTypes } from '../../../../shared/models/result-types';

const resultMapping: { [key: string]: number } = {
  [ResultTypes.FAILED]: 0,
  [ResultTypes.WARNING]: 1,
  [ResultTypes.PASSED]: 2,
};

@Component({
  selector: 'ktb-apex-chart',
  templateUrl: './apex-chart.component.html',
  styleUrls: ['./apex-chart.component.scss'],
})
export class ApexChartComponent {
  public chart: ApexChart;
  public series: ApexAxisChartSeries;
  public plotOptions: ApexPlotOptions;
  public dataLabels: ApexDataLabels;
  public tooltip: ApexTooltip;
  // public xAxis: ApexXAxis;
  public annotations: ApexAnnotations;

  constructor() {
    this.series = this.generateData(Evaluations.data.evaluationHistory as Trace[]) || this.getDefaultData(20);
    // Why don't we take the score and its threshold to determine the color?
    //   Because the threshold could change and would not apply to all evaluations
    this.plotOptions = {
      heatmap: {
        // shadeIntensity: 1,
        // we can't set the right color. It is impossible.
        // With shadeIntensity set to 0, the failed color is completely gone and with 1 the color for warning and success is completely different
        colorScale: {
          ranges: [
            {
              from: resultMapping[ResultTypes.FAILED],
              to: resultMapping[ResultTypes.FAILED],
              name: 'failed',
              color: '#dc172a',
            },
            {
              from: resultMapping[ResultTypes.WARNING],
              to: resultMapping[ResultTypes.WARNING],
              name: 'warning',
              color: '#e6be00',
            },
            {
              from: resultMapping[ResultTypes.PASSED],
              to: resultMapping[ResultTypes.PASSED],
              name: 'succeeded',
              color: '#7dc540',
            },
          ],
        },
      },
    };
    this.chart = {
      type: 'heatmap',
      height: this.series.length * 100, // 100px per row
      selection: {
        enabled: true,
        type: 'y',
      },
      animations: {
        enabled: false,
        easing: 'easeinout',
        speed: 800,
        animateGradually: {
          enabled: false,
          delay: 150,
        },
        dynamicAnimation: {
          enabled: false,
          speed: 350,
        },
      },
      events: {
        click(event: MouseEvent, chart?: ApexChart, options?: unknown): void {
          console.log('clicked');
          console.log(event);
          console.log(chart);
          console.log(options);
        },
      },
    };
    this.dataLabels = {
      enabled: false,
    };
    this.tooltip = {
      enabled: true,
      followCursor: false,
      custom({
        series,
        seriesIndex,
        dataPointIndex,
        w,
      }: {
        series: ApexAxisChartSeries;
        seriesIndex: number;
        dataPointIndex: number;
        // eslint-disable-next-line @typescript-eslint/ban-ts-comment
        // @ts-ignore
        // eslint-disable-next-line @typescript-eslint/no-explicit-any
        w: any;
      }): unknown {
        const data = w.globals.initialSeries[seriesIndex].data[dataPointIndex];
        return `<div>${JSON.stringify(data)}</div>`;
      },
    };

    this.annotations = {
      // can be used for secondary index; on selection change: update annotations

      // placement is totally wrong because it starts at the center of the tile
      xaxis: [
        {
          x: '3',
          x2: '4',
          fillColor: 'black',
          // offsetX: -45,
          opacity: 0.5,
        },
        {
          x: '5',
          x2: '6',
          borderWidth: 0,
          fillColor: 'black',
          // offsetX: -45,
          opacity: 0.5,
        },
      ],
    };
  }

  private getDefaultData(counter: number): ApexAxisChartSeries {
    return [
      {
        name: 'Response Time P95',
        data: this.generatePoint(counter),
      },
      {
        name: 'Score',
        data: this.generatePoint(counter),
      },
    ];
  }

  private generatePoint(counter: number): {
    x: string;
    y: number;
    fillColor?: string;
    strokeColor?: string;
    meta?: unknown;
    goals?: unknown;
  }[] {
    const points = [];
    for (let i = 1; i <= counter; ++i) {
      points.push({
        x: `${i}`,
        y: Math.floor(Math.random() * 3),
      });
    }
    return points;
  }

  private generateData(evaluations: Trace[]): ApexAxisChartSeries | undefined {
    // first create dictionary
    // evaluation should be stored in the component. On click on an element in the heatmap the selected one should be fetched via this.evaluations[this.clickedIndex]
    // iterate over all evaluations
    // remember xAxis indicator (date); const labels = []; labels.push(evaluation.time)
    // dict[indicatorResult.metric] = [data1, data2, ...]
    // iterate through keys and add them to series
    const series: ApexAxisChartSeries = [];
    return undefined;
  }
}
