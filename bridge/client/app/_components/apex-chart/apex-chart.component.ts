import { Component } from '@angular/core';
import { ApexAxisChartSeries, ApexChart, ApexDataLabels, ApexPlotOptions, ApexTooltip, ApexXAxis } from 'ng-apexcharts';
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
  public xaxis: ApexXAxis;

  constructor() {
    this.series = this.generateData(Evaluations.data.evaluationHistory as Trace[]) || this.getDefaultData();
    // Why don't we take the score and its threshold to determine the color?
    //   Because the threshold could change and would not apply to all evaluations
    this.plotOptions = {
      heatmap: {
        colorScale: {
          ranges: [
            {
              from: resultMapping[ResultTypes.FAILED],
              to: resultMapping[ResultTypes.FAILED],
              name: 'failed',
              color: '#ff0000',
              foreColor: '#ff0000',
            },
            {
              from: resultMapping[ResultTypes.WARNING],
              to: resultMapping[ResultTypes.WARNING],
              name: 'warning',
              color: '#ffff00',
              foreColor: '#ffff00',
            },
            {
              from: resultMapping[ResultTypes.PASSED],
              to: resultMapping[ResultTypes.PASSED],
              name: 'succeeded',
              color: '#008000',
              foreColor: '#008000',
            },
          ],
        },
      },
    };
    this.chart = {
      type: 'heatmap',
      height: 200,
      selection: {
        enabled: true,
        type: 'xy',
        stroke: {
          width: 1,
          dashArray: 3,
          color: '#24292e',
          opacity: 0.4,
        },
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
      custom({ series, seriesIndex, dataPointIndex, w }): unknown {
        const data = w.globals.initialSeries[seriesIndex].data[dataPointIndex];
        return `<div>${JSON.stringify(data)}</div>`;
      },
    };
    this.xaxis = {
      categories: ['Date1', 'Date2'],
      labels: {
        show: true,
      },
    };
  }

  private getDefaultData(): ApexAxisChartSeries {
    return [
      {
        name: 'Response Time P95',
        data: this.generatePoint(20),
      },
      {
        name: 'Score',
        data: this.generatePoint(20),
      },
    ];
  }

  private generatePoint(counter: number): {
    x: number;
    y: number;
    fillColor?: string;
    strokeColor?: string;
    meta?: unknown;
    goals?: unknown;
  }[] {
    const points = [];
    for (let i = 1; i <= counter; ++i) {
      points.push({
        x: i,
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
